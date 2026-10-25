# Releasing

This file covers the setup that happens outside the code, and how to push a release by hand. Only the repo owner can do the setup steps.

Steps marked **pending** depend on workflow jobs that are not implemented yet: the Homebrew, npm and Chocolatey jobs, and the smoke job (tasks 4 to 7). Do those steps when the named job lands.

## GitHub repository (m10rten/go-canary-cli)

- [ ] Keep the repository public. Attestations, npm provenance and the free arm64 runners depend on it.
- [ ] Set workflow permissions to read and write. Go to Settings > Actions > General > Workflow permissions. `release.yml` asks for `contents: write` (release and prune-nightlies jobs), `id-token: write` and `attestations: write` (attest job). The npm job (pending, task 5) also needs `id-token: write`.
- [ ] Add a tag ruleset. Go to Settings > Rules > Rulesets > New ruleset > New tag ruleset.
  - Target: tags, include pattern `v*`, exclude pattern `v*-nightly.*`.
  - Turn on "Restrict creations". The bypass list has only you, so only you can create `v*` tags by hand.
  - The exclude pattern lets the workflow create and delete nightly tags. The GitHub docs do not list `GITHUB_TOKEN` as a bypass actor, so the workflow cannot get past the rule any other way.
  - A pushed nightly-shaped tag fails validation and publishes nothing.
  - Put the ruleset in place before the first nightly run.

## Homebrew tap (m10rten/homebrew-tap)

The Homebrew job is pending (task 4). Stable releases only.

- [ ] Create the public repository `m10rten/homebrew-tap` with a README.
- [ ] Create a `Formula/` folder. Git does not store empty folders, so it appears with the first formula commit from the release job.
- [ ] Generate an SSH key pair on your machine. Do not commit the private key.
- [ ] Add the public key as a deploy key on `m10rten/homebrew-tap`. Go to Settings > Deploy keys > Add deploy key, and turn on "Allow write access". This key works for this repo only.
- [ ] Add the private key as a secret on `m10rten/go-canary-cli`. Go to Settings > Secrets and variables > Actions > New repository secret. Name: `HOMEBREW_TAP_DEPLOY_KEY`. Value: the full private key, including the header and footer lines.

## npm (@m10rten/go-canary-cli)

The npm job and the bootstrap make target are pending (task 5). The package name is `@m10rten/go-canary-cli`.

- [ ] Confirm that the `@m10rten` scope belongs to you, either as user `m10rten` or as an org you own, and that 2FA is on.
- [ ] Bootstrap-publish version `0.0.0` from a local login with public access. Trusted publishing needs the package to exist first. Installing `0.0.0` fails at postinstall, because there is no `v0.0.0` release. That is expected.
  - Run `make -s release-npm-bootstrap`. It builds the package in `bin/npm/` with the tag fixed to `v0.0.0`. It does not publish.
  - Then run `npm publish ./bin/npm` from a local npm login. The generated package.json sets `publishConfig.access` to `public`, so this first publish is public.
  - This target arrives with the npm task and does not exist yet. The plan names it, but the code is not written.
- [ ] Add a trusted publisher to the package on npmjs.com.
  - Repository: `m10rten/go-canary-cli`
  - Workflow file: `release.yml`
  - Environment: leave empty. `release.yml` does not use a GitHub environment.
  - Turn on "Allow npm dist-tag". npm documents this option, but this repo has not exercised it yet. The first channel releases verify it. Until then, treat it as unconfirmed.
- [ ] Run `npm deprecate` on `0.0.0` from your local login. Trusted publishing covers publishing, and dist-tag commands only with "Allow npm dist-tag" on. Deprecate needs a normal login (inferred from npm's docs, not tested here).
- [ ] Push the first channel release within 2 days of adding the trusted publisher. npm expires a trusted publisher that has not published within 2 days.

After that, the workflow moves the `nightly`, `alpha`, `beta` and `rc` dist-tags over OIDC. `latest` is set by stable releases only.

## Chocolatey (m10rten-canary)

The Chocolatey job is pending (task 6). Stable releases only.

- [ ] Create an account at community.chocolatey.org.
- [ ] Copy the API key from the account page and add it as a secret on `m10rten/go-canary-cli`. Go to Settings > Secrets and variables > Actions > New repository secret. Name: `CHOCOLATEY_API_KEY`.
- The package id is `m10rten-canary`.
- Chocolatey reviews each new version of an untrusted package by hand. A new release can take time to show up in the community feed.

## Pushing a release by hand

Stable, alpha, beta and rc tags are pushed by hand. Nightly tags are created by the workflow.

- Stable: `vX.Y.Z`, for example `v0.2.0`. Works now.
- Prerelease: `vX.Y.Z-alpha.N`, `vX.Y.Z-beta.N` or `vX.Y.Z-rc.N`, for example `v0.2.0-beta.1`. N is 1 or higher, with no leading zeros. Published as a prerelease, not as latest.
- Do not push a `-nightly.` tag. It fails validation and publishes nothing.

```
git tag v0.2.0
git push origin v0.2.0
```

Nightly releases run in two ways:

- On a daily schedule at 03:17 UTC (cron `17 3 * * *`).
- By hand, with workflow_dispatch on `main`. The optional `keep` input sets how many nightlies to keep (default 14, must be 1 or more). Lower it only to test retention.

Both run from `main` only. A run from any other branch fails.

A nightly run works like this:

- It builds when the head of `main` differs from the commit of the newest nightly. If `main` has not moved and that nightly has no published release, the run resumes it. Otherwise it ends green with a "no changes" notice and creates nothing.
- A run whose commit is no longer the head of `main` fails. Re-runs of old runs are refused.
- It makes one nightly per UTC day. If today's nightly tag exists on a different commit, the run fails. To build that day again from `main`, delete that release and its tag.
- Its tag is `vX.Y.Z-nightly.YYYYMMDD`. The base is the highest stable tag, with the patch number plus one.
- It keeps the newest 14 nightly releases by default and deletes older ones. Pruning runs only when a new nightly publishes, so days with no changes do not prune.
- GitHub disables scheduled runs in a public repository after 60 days without activity.

If a release run fails partway, use "Re-run failed jobs" on the original run. "Re-run all jobs" stops with an error for a release that is already published. "Re-run failed jobs" is also safe after the nightly tag was created.

## Known limitations

- Push prerelease and stable tags one at a time. The release runs share one concurrency group, so a second push while one run is waiting can drop the earlier pending run. If that happens, re-run the dropped run or push the tag again.
- Nightly tags are not covered by the ruleset, so anyone with write access can push a `v*-nightly.*` tag. Such a push fails validation and publishes nothing. A future-dated one blocks real nightlies until it is deleted.
- Old nightlies are pruned only when a new nightly publishes. Days with no changes do not prune.
- Not yet confirmed on GitHub: whether a tag created by the workflow with `GITHUB_TOKEN` starts a second `push` run. If it does, that run fails at tag validation without releasing anything.
