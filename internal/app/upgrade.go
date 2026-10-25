package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/m10rten/go-canary-cli/internal/channel"
	"github.com/m10rten/go-canary-cli/internal/cli"
	"github.com/m10rten/go-canary-cli/internal/release"
	"github.com/m10rten/go-canary-cli/internal/term"
)

type upgradeAction int

const (
	printInstruction upgradeAction = iota + 1
	upgradeInPlace
)

// upgradePlan is the outcome of routing an upgrade. For printInstruction, text
// is written to stdout. For upgradeInPlace, target is the normalized tag, or
// empty for the latest release.
type upgradePlan struct {
	action upgradeAction
	text   string
	target string
}

const reinstallHint = "reinstall canary with install.sh"

// planUpgrade decides what `canary upgrade` does for a channel and an
// optional --version value. distTag is the npm dist-tag of the running build's
// track. A returned error means exit code 2.
func planUpgrade(ch channel.Channel, distTag, versionFlag string) (upgradePlan, error) {
	if ch == channel.InstallSh {
		target, err := normalizeTag(versionFlag)
		if err != nil {
			return upgradePlan{}, err
		}
		return upgradePlan{action: upgradeInPlace, target: target}, nil
	}

	command, managed := channel.UpgradeCommandFor(ch, distTag)
	if versionFlag != "" {
		return upgradePlan{}, fmt.Errorf("--version only works for install.sh installs, but this install is %s", describeUnmanaged(ch, command))
	}
	if managed {
		return upgradePlan{action: printInstruction, text: fmt.Sprintf("canary is managed by %s. To upgrade, run:\n  %s\n", ch, command)}, nil
	}
	return upgradePlan{}, fmt.Errorf("cannot upgrade: install channel is unknown; %s", reinstallHint)
}

func describeUnmanaged(ch channel.Channel, command string) string {
	if command == "" {
		return "of unknown origin; " + reinstallHint
	}
	return fmt.Sprintf("managed by %s (to upgrade, run: %s)", ch, command)
}

// normalizeTag returns tag with a leading "v", or "" when tag is empty.
func normalizeTag(tag string) (string, error) {
	if tag == "" {
		return "", nil
	}
	if _, ok := release.ParseVersion(tag); !ok {
		return "", &cli.UsageError{Kind: cli.InvalidValue, Msg: fmt.Sprintf("invalid --version %q: want a release tag such as v0.2.0", tag)}
	}
	return "v" + strings.TrimPrefix(tag, "v"), nil
}

func (a *App) upgrade(ctx *cli.Context) error {
	got, err := a.installChannel()
	if err != nil {
		return err
	}
	plan, err := planUpgrade(got.Channel, release.TrackOf(a.Build.Version).DistTag(), ctx.String("version"))
	if err != nil {
		return err
	}
	if plan.action == printInstruction {
		_, err = fmt.Fprint(a.Stdout, plan.text)
		return err
	}
	return a.upgradeInPlace(ctx, got.Path, plan.target)
}

const (
	downloadTimeout   = 5 * time.Minute
	maxChecksumsBytes = 1 << 20
	maxArchiveBytes   = 200 << 20
)

// upgradeInPlace replaces the executable at path with the release tagged
// target (latest when empty). The checksum is verified before the installed
// binary is touched.
func (a *App) upgradeInPlace(ctx *cli.Context, path, target string) error {
	if a.Build.OS != "darwin" && a.Build.OS != "linux" {
		return fmt.Errorf("self-upgrade is not supported on %s", a.Build.OS)
	}
	client := a.GitHub
	if token, ok := a.lookupEnv("GITHUB_TOKEN"); ok {
		client.Token = token
	}
	reporter := a.reporter(ctx)

	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	latest := target == ""
	if latest {
		task := reporter.Spin("Resolving latest version")
		tag, err := latestRelease(runCtx, client, a.Build.Version)
		if err != nil {
			task.Fail("Could not resolve the latest version")
			return fmt.Errorf("look up latest release: %w", err)
		}
		task.Done("Latest release is " + tag)
		target = tag
	}

	order := release.Compare(a.Build.Version, target)
	if order == 0 || (latest && order > 0) {
		_, err := fmt.Fprintf(a.Stdout, "Already up to date (%s)\n", a.Build.Version)
		return err
	}

	archiveName := release.ArchiveName(target, a.Build.OS, a.Build.Arch)
	checksums, err := download(runCtx, client, reporter, target, release.ChecksumsName, maxChecksumsBytes)
	if err != nil {
		return err
	}
	archive, err := download(runCtx, client, reporter, target, archiveName, maxArchiveBytes)
	if err != nil {
		return err
	}

	task := reporter.Spin("Verifying checksum")
	if err := release.VerifyChecksum(checksums, archiveName, bytes.NewReader(archive)); err != nil {
		task.Fail("Checksum verification failed")
		return err
	}
	task.Done("Checksum verified")

	task = reporter.Spin("Installing " + target)
	binary, err := release.ExtractBinary(archive, release.BinaryName)
	if err == nil {
		err = release.ReplaceBinary(path, binary)
	}
	if err != nil {
		task.Fail("Could not install " + target)
		return err
	}
	task.Done("Installed " + path)

	verb := "Upgraded"
	if order > 0 {
		verb = "Downgraded"
	}
	_, err = fmt.Fprintf(a.Stdout, "%s canary %s → %s\n", verb, a.Build.Version, target)
	return err
}

// download reads a release asset into memory, showing a progress bar.
func download(ctx context.Context, client release.Client, reporter term.Reporter, tag, asset string, limit int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()

	body, size, err := client.OpenAsset(ctx, tag, asset)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	task := reporter.Bar("Downloading "+asset, size)
	data, err := io.ReadAll(io.LimitReader(&progressReader{r: body, update: task.Update}, limit+1))
	if err == nil && int64(len(data)) > limit {
		err = fmt.Errorf("%s exceeds %d MB", asset, limit>>20)
	}
	if err != nil {
		task.Fail("Could not download " + asset)
		return nil, fmt.Errorf("download %s: %w", asset, err)
	}
	task.Done("Downloaded " + asset)
	return data, nil
}

// progressReader reports the running byte count to update on every read.
type progressReader struct {
	r      io.Reader
	done   int64
	update func(done int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	p.update(p.done)
	return n, err
}
