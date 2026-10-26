# Installs canary from a GitHub release on Windows, like install.sh does on Linux and macOS:
# resolve the tag, download the .zip and checksums.txt, verify the SHA-256, install
# canary.exe. The go-canary-cli alias is not created, since it is a symlink.
#
#   CANARY_VERSION         release tag to install (default: latest release)
#   CANARY_TRACK           stable (default), nightly, alpha, beta or rc: installs the newest
#                          release on that track's upgrade path, like install.sh.
#                          CANARY_VERSION takes precedence.
#   CANARY_INSTALL_DIR     install directory (default: %LOCALAPPDATA%\canary\bin)
#   CANARY_NO_MODIFY_PATH  set to 1 to not add the install directory to your user PATH
#                          (default: it is added, unless it is already on it)
#   CANARY_GITHUB_TOKEN    GitHub token for the release lookup, to stay clear of the API
#                          rate limit (default: none). Sent as a bearer token only to
#                          https://api.github.com/, never with the asset downloads.
#
# CANARY_DOWNLOAD_BASE_URL is not supported here.
# Written for Windows PowerShell 5.1, so it also runs under pwsh. Run it from a file, or
# with: irm <url of this file> | iex
# The body is a script block so that none of its settings or functions leak into the
# session when it is piped to iex.

& {
    $ErrorActionPreference = 'Stop'
    $ProgressPreference = 'SilentlyContinue'
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

    $repo = 'm10rten/go-canary-cli'
    $apiHint = 'This may be a GitHub API rate limit. Set CANARY_VERSION to install a specific tag.'
    $trackPaths = @{
        stable  = @('stable')
        nightly = @('nightly', 'stable')
        rc      = @('rc', 'stable')
        beta    = @('beta', 'rc', 'stable')
        alpha   = @('alpha', 'beta', 'rc', 'stable')
    }
    $ranks = @{ alpha = 1; beta = 2; rc = 3; nightly = 4; stable = 9 }
    $tagPattern = '^v(?<major>0|[1-9][0-9]*)\.(?<minor>0|[1-9][0-9]*)\.(?<patch>0|[1-9][0-9]*)(?:-(?:(?<pre>alpha|beta|rc)\.(?<n>[1-9][0-9]*)|(?<pre>nightly)\.(?<n>[0-9]{8})))?\z'
    $pinnedPattern = '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?\z'

    function Get-ApiJson([string]$Url) {
        $headers = @{ Accept = 'application/vnd.github+json'; 'User-Agent' = 'canary-install' }
        if ($env:CANARY_GITHUB_TOKEN -and $Url.StartsWith('https://api.github.com/')) {
            $headers.Authorization = "Bearer $($env:CANARY_GITHUB_TOKEN)"
        }
        try {
            Invoke-RestMethod -Uri $Url -Headers $headers
        } catch {
            throw "download failed: $Url. $apiHint ($($_.Exception.Message))"
        }
    }

    function Get-File([string]$Url, [string]$Path) {
        try {
            Invoke-WebRequest -Uri $Url -OutFile $Path -UseBasicParsing
        } catch {
            throw "download failed: $Url ($($_.Exception.Message))"
        }
    }

    function Resolve-Tag {
        $track = if ($env:CANARY_TRACK) { $env:CANARY_TRACK } else { 'stable' }
        if (-not $trackPaths.ContainsKey($track) -or $track -cne $track.ToLowerInvariant()) {
            throw "CANARY_TRACK must be one of stable, nightly, alpha, beta or rc, got: $track"
        }
        if ($env:CANARY_VERSION) {
            if ($env:CANARY_VERSION -cnotmatch $pinnedPattern) {
                throw "CANARY_VERSION must be a release tag like v1.2.3, got: $($env:CANARY_VERSION)"
            }
            return $env:CANARY_VERSION
        }
        if ($track -eq 'stable') {
            $latest = (Get-ApiJson "https://api.github.com/repos/$repo/releases/latest").tag_name
            if ($latest -cnotmatch '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\z') {
                throw "could not determine the latest release (got '$latest'). Set CANARY_VERSION to install a specific tag."
            }
            return $latest
        }
        $candidates = foreach ($release in @(Get-ApiJson "https://api.github.com/repos/$repo/releases?per_page=100")) {
            if ($release.draft -or -not ($release.tag_name -cmatch $tagPattern)) { continue }
            $name = if ($Matches.pre) { $Matches.pre } else { 'stable' }
            if ($trackPaths[$track] -notcontains $name) { continue }
            [pscustomobject]@{
                Tag   = $release.tag_name
                Major = [Numerics.BigInteger]::Parse($Matches.major)
                Minor = [Numerics.BigInteger]::Parse($Matches.minor)
                Patch = [Numerics.BigInteger]::Parse($Matches.patch)
                Rank  = $ranks[$name]
                N     = if ($Matches.n) { [Numerics.BigInteger]::Parse($Matches.n) } else { [Numerics.BigInteger]::Zero }
            }
        }
        $best = @($candidates | Sort-Object Major, Minor, Patch, Rank, N -Descending | Select-Object -First 1)
        if ($best.Count -eq 0) { throw "no release found on the $track track. Set CANARY_VERSION to install a specific tag." }
        return $best[0].Tag
    }

    function Assert-Checksum([string]$File, [string]$Name, [string]$ChecksumsFile) {
        $wanted = @(Get-Content -LiteralPath $ChecksumsFile | ForEach-Object {
                $fields = ($_ -replace "`r", '').Trim() -split '\s+'
                if ($fields.Count -ge 2 -and ($fields[1] -ceq $Name -or $fields[1] -ceq "*$Name")) { $fields[0].ToLowerInvariant() }
            })
        if ($wanted.Count -eq 0) { throw "checksums.txt has no line for $Name. Nothing was installed." }
        if ($wanted.Count -ne 1 -or $wanted[0] -cnotmatch '^[0-9a-f]{64}$') {
            throw "checksums.txt has a duplicate or malformed line for $Name. Nothing was installed."
        }
        $got = (Get-FileHash -LiteralPath $File -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($got -ne $wanted[0]) { throw "checksum mismatch for $Name (expected $($wanted[0]), got $got). Nothing was installed." }
    }

    function Test-PathEntry([string]$List, [string]$Dir) {
        foreach ($entry in $List -split ';') {
            if ([Environment]::ExpandEnvironmentVariables($entry.Trim()).TrimEnd('\', '/') -eq $Dir) { return $true }
        }
        return $false
    }

    # Tells running programs, Explorer included, to reread the environment. Best effort.
    function Send-EnvironmentChange {
        try {
            if (-not ('Canary.NativeMethods' -as [type])) {
                Add-Type -Namespace Canary -Name NativeMethods -MemberDefinition '[DllImport("user32.dll", SetLastError = true, CharSet = CharSet.Auto)] public static extern IntPtr SendMessageTimeout(IntPtr hWnd, uint Msg, UIntPtr wParam, string lParam, uint fuFlags, uint uTimeout, out UIntPtr lpdwResult);'
            }
            $result = [UIntPtr]::Zero
            # HWND_BROADCAST, WM_SETTINGCHANGE, SMTO_ABORTIFHUNG
            [void][Canary.NativeMethods]::SendMessageTimeout([IntPtr]0xffff, 0x1A, [UIntPtr]::Zero, 'Environment', 2, 5000, [ref]$result)
        } catch {
            Write-Host "Could not tell running programs about the PATH change; open a new terminal."
        }
    }

    # Edits the registry value directly so %VAR% entries stay unexpanded and the value kind is kept.
    # Writes only when the directory is missing from the user PATH, so a rerun changes nothing.
    function Add-ToUserPath([string]$Dir) {
        $key = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey('Environment')
        try {
            $raw = [string]$key.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
            if (Test-PathEntry $raw $Dir) {
                Write-Host "$Dir is already on your PATH"
            } else {
                $kind = if ($key.GetValueNames() -contains 'Path') { $key.GetValueKind('Path') } else { [Microsoft.Win32.RegistryValueKind]::ExpandString }
                $updated = if ($raw) { "$($raw.TrimEnd(';'));$Dir" } else { $Dir }
                $key.SetValue('Path', $updated, $kind)
                Send-EnvironmentChange
                Write-Host "Added $Dir to your user PATH"
            }
        } finally {
            $key.Close()
        }
        if (-not (Test-PathEntry $env:Path $Dir)) { $env:Path = "$Dir;$env:Path" }
    }

    $work = $null
    try {
        $arch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
        $goarch = switch ($arch) {
            'AMD64' { 'amd64' }
            'ARM64' { 'arm64' }
            default { throw "unsupported architecture: $arch" }
        }

        $dir = $env:CANARY_INSTALL_DIR
        if (-not $dir) {
            if (-not $env:LOCALAPPDATA) { throw 'LOCALAPPDATA is not set; set CANARY_INSTALL_DIR' }
            $dir = Join-Path $env:LOCALAPPDATA 'canary\bin'
        }
        if (-not [IO.Path]::IsPathRooted($dir)) { $dir = Join-Path (Get-Location).Path $dir }
        $dir = [IO.Path]::GetFullPath($dir).TrimEnd('\', '/')

        $tag = Resolve-Tag
        $archive = "canary_$($tag.Substring(1))_windows_$goarch.zip"
        $base = "https://github.com/$repo/releases/download/$tag"

        $work = Join-Path ([IO.Path]::GetTempPath()) "canary-install-$([guid]::NewGuid().ToString('N'))"
        New-Item -ItemType Directory -Path $work | Out-Null

        Write-Host "Downloading canary $tag for windows/$goarch"
        Get-File "$base/$archive" (Join-Path $work $archive)
        Get-File "$base/checksums.txt" (Join-Path $work 'checksums.txt')
        Assert-Checksum (Join-Path $work $archive) $archive (Join-Path $work 'checksums.txt')

        $extract = Join-Path $work 'extract'
        Expand-Archive -LiteralPath (Join-Path $work $archive) -DestinationPath $extract
        $exe = Join-Path $extract 'canary.exe'
        if (-not (Test-Path -LiteralPath $exe -PathType Leaf)) { throw "$archive does not hold a canary.exe at its root" }

        New-Item -ItemType Directory -Path $dir -Force | Out-Null
        $target = Join-Path $dir 'canary.exe'
        if (Test-Path -LiteralPath $target -PathType Container) { throw "$target is a directory" }
        $staged = Join-Path $dir ".canary-$([guid]::NewGuid().ToString('N')).exe"
        Copy-Item -LiteralPath $exe -Destination $staged
        Move-Item -LiteralPath $staged -Destination $target -Force

        Write-Host "Installed canary $tag to $dir"
        if ($env:CANARY_NO_MODIFY_PATH -ne '1') { Add-ToUserPath $dir }
    } catch {
        [Console]::Error.WriteLine("install.ps1: error: $($_.Exception.Message)")
        # exit would close the session of someone running this through iex.
        if ($PSCommandPath) { exit 1 }
    } finally {
        if ($work -and (Test-Path -LiteralPath $work)) { Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue }
    }
}
