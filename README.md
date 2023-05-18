# semver-next

__semver-next__ is a command-line utility that automates release versions for your GitHub project.

## How it works

When looking at pull requests, PRs labeled with `breaking` or `breaking change` will cause the major version to be
incremented. PRs labeled with `enhancement` will increment the minor version, and PRs with none of those labels will
increment the patch version. If there are multiple PRs, or multiple conflicting labels on a PR, the highest version bump
wins.

semver-next also looks at commit messages and evaluates their prefixes based on the
[Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) specification. Commits in a PR are evaluated
separately from the PR's labels. Whichever results in a bigger version change will be used.

Your local git clone is not used by semver-next. Instead it gets all PR data and commit messages through the GitHub API.
Because of this, you do need to set the GITHUB_TOKEN environment variable so that semver-next can authenticate with
GitHub.

## Usage

```
Usage: semver-next --ref=STRING --prev-ref=STRING <repo>

semver-next will analyze the merged pull requests and commits since a GitHub repository's latest
release to determine the next release version based on pull request labels.

Arguments:
  <repo>    GitHub repository in "<owner>/<repo>" format. e.g. WillAbides/semver-next

Flags:
  -h, --help                   Show context-sensitive help.
  -r, --ref=STRING             The tag, branch or commit sha that will be tagged for the next
                               release.
  -p, --prev-ref=STRING        The git tag from the previous release. This should rarely be needed.
                               When this is unset, it uses the tag of the release marked "latest
                               release" on the GitHub releases page.
  -v, --prev-version=STRING    The version of the previous release in semver format. This may be
                               necessary when release tags don't follow semver format.
      --max-bump="major"       The maximum amount to bump the version.
      --min-bump="none"        The maximum amount to bump the version.
      --version                output semver-next's version and exit
      --json                   Output in JSON format
```
