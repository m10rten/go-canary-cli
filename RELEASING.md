# Releasing

This file covers the setup that happens outside the code, and how to push a release. Only the repo owner can do the setup steps.

Steps marked "(pending, task N)" depend on workflow jobs or make targets that are not built yet. Task numbers refer to `.claude/plans/pass-2-distribution/plan.md`. The unmarked GitHub steps work today. The pending pieces are:

- Jobs: `publish-npm` (task 5), `publish-homebrew` (task 4), `publish-chocolatey` (task 6) and the smoke job (task 7).
- Make targets: `release-npm-package` and `release-npm-bootstrap` (task 5).

## Channels and tracks

Every release has a track, and the track decides which channels carry it. The npm dist-tag matches the track name, except stable, which uses `latest`.

| Track   | Tag                       | GitHub release          | npm dist-tag | Homebrew | Chocolatey | install.sh                    |
| ------- | ------------------------- | ----------------------- | ------------ | -------- | ---------- | ----------------------------- |
| stable  | `vX.Y.Z`                  | full release, latest    | `latest`     | yes      | yes        | default                       |
| rc      | `vX.Y.Z-rc.N`             | prerelease, not latest  | `rc`         | no       | no         | `CANARY_TRACK=rc`             |
| beta    | `vX.Y.Z-beta.N`           | prerelease, not latest  | `beta`       | no       | no         | `CANARY_TRACK=beta`           |
| alpha   | `vX.Y.Z-alpha.N`          | prerelease, not latest  | `alpha`      | no       | no         | `CANARY_TRACK=alpha`          |
| nightly | `vX.Y.Z-nightly.YYYYMMDD` | prerelease, not latest  | `nightly`    | no       | no         | `CANARY_TRACK=nightly`        |

Homebrew and Chocolatey carry stable only (pending, tasks 4 and 6). Chocolatey reviews each new version of an untrusted package by hand, so daily nightlies would queue a review every day. Homebrew has no prerelease convention for taps.

A stable release keeps `releases/latest` pointing at it. Prereleases never move `latest`.

## GitHub repository (m10rten/go-canary-cli)

- [ ] Keep the repository public. Attestations, npm provenance and the free arm64 runners depend on it.
- [ ] Set workflow permissions to read and write. Go to Settings > Actions > General > Workflow permissions. `release.yml` asks for `contents: write` (release and prune-nightlies jobs), `id-token: write` and `attestations: write` (attest job). The npm job (pending, task 5) also needs `id-token: write`.
- [ ] Add a tag ruleset. Go to Settings > Rules > Rulesets > New ruleset > New tag ruleset.
  - Target: tags, include pattern `v*`, exclude pattern `v*-nightly.*`.
  - Turn on "Restrict creations". The bypass list has only you, so only you can create `v*` tags by hand.
  - The exclude pattern lets the workflow create and delete nightly tags. The GitHub docs do not list `GITHUB_TOKEN` as a bypass actor, so the workflow cannot get past the rule any other way.
  - A pushed nightly-shaped tag fails validation and publishes nothing.
  - Put the ruleset in place before the first nightly run.

No job uses a GitHub environment. There is nothing to create, and the npm trusted publisher's environment field stays empty.

### Secrets and scopes

Add repository secrets under Settings > Secrets and variables > Actions > New repository secret.

| Name                      | Type         | Scope                                                                                              | Used by                              |
| ------------------------- | ------------ | -------------------------------------------------------------------------------------------------- | ------------------------------------ |
| `GITHUB_TOKEN`            | built in     | Per job, set in `release.yml`: `contents: write` (release, prune), `id-token` and `attestations: write` (attest) | release workflow                     |
| `HOMEBREW_TAP_DEPLOY_KEY` | repo secret  | Write access to `m10rten/homebrew-tap` only, through a deploy key                                  | publish-homebrew (pending, task 4)   |
| `CHOCOLATEY_API_KEY`      | repo secret  | Pushes packages as the Chocolatey account the key belongs to. The plan does not narrow it further  | publish-chocolatey (pending, task 6) |
| npm                       | none         | OIDC trusted publishing from `release.yml`. No stored token                                        | publish-npm (pending, task 5)        |

## Homebrew tap (m10rten/homebrew-tap)

The Homebrew job is pending (task 4). Stable releases only.

The repo is `m10rten/homebrew-tap`. Users tap it as `m10rten/tap`, so the formula is `m10rten/tap/canary`. Homebrew drops the `homebrew-` prefix when it taps a repo.

- [ ] Create the public repository `m10rten/homebrew-tap` with a README.
- [ ] Create a `Formula/` folder. Git does not store empty folders, so it appears with the first formula commit from the release job (pending, task 4). Do not commit an empty placeholder.
- [ ] Generate an SSH key pair on your machine. Do not commit the private key.
- [ ] Add the public key as a deploy key on `m10rten/homebrew-tap`. Go to Settings > Deploy keys > Add deploy key, and turn on "Allow write access". This key works for this repo only.
- [ ] Add the private key as the secret `HOMEBREW_TAP_DEPLOY_KEY`. Value: the full private key, including the header and footer lines.

Automatic (pending, task 4): on a stable tag, `publish-homebrew` renders `Formula/canary.rb` from the tag and `checksums.txt`, and pushes one commit to the tap with the deploy key. If the formula already has this version, the job makes no commit.

Verify (pending, task 4):

```
brew update
brew info m10rten/tap/canary
```

The output shows the new version.

Recovery (pending, task 4):

- Re-run failed jobs on the original run. The job is a no-op when the formula already has the version.
- A re-run does not fix a formula that was pushed wrong, because the version already matches. Fix the formula with a commit to the tap, or publish the next patch version.

## npm (@m10rten/go-canary-cli)

The npm job and the bootstrap make targets are pending (task 5). The package name is `@m10rten/go-canary-cli`.

- [ ] Confirm that the `@m10rten` scope belongs to you, either as user `m10rten` or as an org you own, and that 2FA is on.
- [ ] Bootstrap-publish version `0.0.0` from a local login with public access. Trusted publishing needs the package to exist first. Installing `0.0.0` fails at postinstall, because there is no `v0.0.0` release. That is expected.
  - Run `make -s release-npm-bootstrap` (pending, task 5). It builds the package in `bin/npm/` with the tag fixed to `v0.0.0`. It does not publish.
  - Then run `npm publish ./bin/npm` from a local npm login. The generated package.json sets `publishConfig.access` to `public`, so this first publish is public.
  - The bootstrap publish has no `--tag`, so npm sets `latest` to `0.0.0`. `npm install -g @m10rten/go-canary-cli` fails at postinstall until the first stable release moves `latest`.
- [ ] Add a trusted publisher to the package on npmjs.com.
  - Repository: `m10rten/go-canary-cli`
  - Workflow file: `release.yml`
  - Environment: leave empty. `release.yml` does not use a GitHub environment.
  - Turn on "Allow npm dist-tag". npm documents this option, but this repo has not exercised it yet. The first channel releases verify it (pending, task 5). Until then, treat it as unconfirmed.
- [ ] Run `npm deprecate` on `0.0.0` from your local login. Trusted publishing covers publishing, and dist-tag commands only with "Allow npm dist-tag" on. Deprecate needs a normal login (inferred from npm's docs, not tested here).
- [ ] Push the first channel release within 2 days of adding the trusted publisher. npm expires a trusted publisher that has not published within 2 days. A nightly run with no changes on `main` publishes nothing, so it does not count.

The publish job (pending, task 5) sets Node 22.14.0 or later and npm 11.5.1 or later itself. Nothing to install by hand.

Automatic (pending, task 5), on every release tag and every nightly:

1. Stages the package for the tag in `bin/npm/`. No `package.json` is checked in.
2. Publishes with `--tag` set to the track's dist-tag: `latest` for stable, otherwise `nightly`, `alpha`, `beta` or `rc`. Auth and provenance come from OIDC, with no token.
3. Skips the publish if that version already exists on npm.
4. Runs the dist-tag reconcile. Each existing `nightly`, `alpha`, `beta` and `rc` tag moves to the highest release in its track's path. A tag that does not exist yet is not created. `latest` is never changed here.

After a stable release, the existing non-stable dist-tags point at that stable, until a newer release on their track exists. So `@nightly` installs the stable when no newer nightly has been built.

Verify (pending, task 5):

```
npm view @m10rten/go-canary-cli dist-tags
npm view @m10rten/go-canary-cli@nightly version
```

On npmjs.com, the package page shows provenance linked to `m10rten/go-canary-cli`.

Recovery (pending, task 5):

- Re-run failed jobs. A re-run skips a version that is already published, and the dist-tag reconcile still runs.
- npm versions are unique, so a bad version is replaced by a new version, not by republishing. Options:
  - Publish the next patch version, or the next prerelease number.
  - `npm deprecate` the bad version from a normal npm login. Trusted publishing does not cover this (inferred from npm's docs, not tested here).
  - `npm unpublish` the bad version from a normal npm login. npm allows it within 72 hours if nothing depends on the version. After 72 hours, it also needs no dependents, under 300 downloads in the last week and a single owner.
- A wrong dist-tag other than `latest` is fixed by re-running the publish job, because the reconcile moves it back. `latest` is not reconciled. Fixing it by hand needs `npm dist-tag add` from a normal npm login (inferred, not tested here).
- Once a version is unpublished, the plan does not cover publishing that version number again. Use a new version.

## Chocolatey (m10rten-canary)

The Chocolatey job is pending (task 6). Stable releases only.

- [ ] Create an account at community.chocolatey.org.
- [ ] Copy the API key from the account page and add it as the secret `CHOCOLATEY_API_KEY`.
- The package id is `m10rten-canary`. The first successful push creates the package. Check the package owner on community.chocolatey.org after the first push (open item, task 6). The plan does not describe an ownership step.
- Chocolatey reviews each new version of an untrusted package by hand. A new release can take time to show up in the community feed.

Automatic (pending, task 6): on a stable tag, `publish-chocolatey` runs on `windows-latest`, builds the package from the tag and `checksums.txt`, and pushes it to `https://push.chocolatey.org/` with `CHOCOLATEY_API_KEY`. An HTTP 409 on push means this version is already on the feed, and the job counts it as success.

Verify (pending, task 6):

```
choco search m10rten-canary --exact --all-versions
```

The community feed can lag behind the push while the version is in review. The package page is at `https://community.chocolatey.org/packages/m10rten-canary`.

Recovery (pending, task 6):

- Re-run failed jobs. A 409 counts as success.
- The Chocolatey cells of the smoke workflow only pass once the version is approved. After approval, re-run the smoke workflow by hand (pending, task 7).
- A pushed version number cannot be pushed again, so a bad package needs a new version. Moderation is done by Chocolatey, not by this repo. The plan has no step for a rejected version, and it does not cover unlisting or removing a version. Check the Chocolatey docs before relying on either.

## Pushing a release

Stable, alpha, beta and rc tags are pushed by hand. Nightly tags are created by the workflow.

- Stable: `vX.Y.Z`, for example `v0.2.0`. Works now.
- Prerelease: `vX.Y.Z-alpha.N`, `vX.Y.Z-beta.N` or `vX.Y.Z-rc.N`, for example `v0.2.0-beta.1`. N is 1 or higher, with no leading zeros. Published as a prerelease, not as latest. The GitHub side works now. The npm dist-tag is pending (task 5).
- Do not push a `-nightly.` tag. It fails validation and publishes nothing.

```
git tag v0.2.0
git push origin v0.2.0
```

Per release:

1. Push the tag.
2. Watch the run. The `resolve`, `build`, `attest` and `release` jobs run first and work today. The release is public after the `release` job. The channel jobs (`publish-npm`, `publish-homebrew` for stable, `publish-chocolatey` for stable) and `smoke` run after it and are pending.
3. Verify the GitHub release and each channel with the commands in the sections above.
4. For stable, after Chocolatey approval, re-run the smoke workflow by hand (pending, task 7).
5. If a run fails, use "Re-run failed jobs" on the original run.

Verify GitHub:

```
gh api repos/m10rten/go-canary-cli/releases/latest --jq .tag_name
gh attestation verify canary_0.2.0_linux_amd64.tar.gz -R m10rten/go-canary-cli
```

The first command prints the newest stable tag. A prerelease must not change it. The second command checks the attestation of an archive. Do the same for `checksums.txt`.

Nightly releases run in two ways:

- On a daily schedule at 03:17 UTC (cron `17 3 * * *`).
- By hand, with workflow_dispatch on `main`. For example, `gh workflow run release.yml --ref main`. The optional `keep` input sets how many nightlies to keep (default 14, must be 1 or more). Lower it only to test retention, for example `-f keep=3`.

Both run from `main` only. A run from any other branch fails.

A nightly run works like this:

- It builds when the head of `main` differs from the commit of the newest nightly. If `main` has not moved and that nightly has no published release, the run resumes it. Otherwise it ends green with a "no changes" notice and creates nothing.
- A run whose commit is no longer the head of `main` fails. Re-runs of old runs are refused.
- It makes one nightly per UTC day. If today's nightly tag exists on a different commit, the run fails. To build that day again from `main`, delete that release and its tag.
- Its tag is `vX.Y.Z-nightly.YYYYMMDD`. The base is the highest stable tag, with the patch number plus one.
- It keeps the newest 14 nightly releases by default and deletes older ones. Pruning runs only when a new nightly publishes, so days with no changes do not prune.
- npm keeps old nightly versions. Once their GitHub release is pruned, they fail at postinstall with the existing clear error. The dist-tags never point at them.
- The npm publish and dist-tag steps of a nightly are pending (task 5). The nightly smoke is pending (task 7).
- GitHub disables scheduled runs in a public repository after 60 days without activity.

If a release run fails partway, use "Re-run failed jobs" on the original run. "Re-run all jobs" stops with an error for a release that is already published. "Re-run failed jobs" is also safe after the nightly tag was created.

## Known limitations

- Tag pushes share one concurrency group, so a second tag push while one run is waiting can drop the earlier pending run. If that happens, re-run the dropped run or push the tag again. Nightlies use their own group, so they never drop a tag push.
- Nightly tags are not covered by the ruleset, so anyone with write access can push a `v*-nightly.*` tag. Such a push fails validation and publishes nothing. A future-dated one blocks real nightlies until it is deleted.
- Old nightlies are pruned only when a new nightly publishes. Days with no changes do not prune.
- Not yet confirmed on GitHub: whether a tag created by the workflow with `GITHUB_TOKEN` starts a second `push` run. If it does, that run fails at tag validation without releasing anything.
- Not yet confirmed: "Allow npm dist-tag" (task 5). If the reconcile cannot move dist-tags, `@nightly` stays at an older version between a stable release and the next nightly.
