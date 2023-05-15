package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/alecthomas/kong"
	"github.com/google/go-github/v52/github"
	"golang.org/x/oauth2"
)

var version = "unknown"

var kongVars = kong.Vars{
	"repo_help": `GitHub repository in "<owner>/<repo>" format. e.g. WillAbides/semver-next`,

	"prev_tag_help": `The git tag from the previous release. This should rarely be needed. When this is unset, it uses 
the tag of the release marked "latest release" on the GitHub releases page.`,

	"prev_version_help": `The version of the previous release in semver format. This may be necessary when release tags 
don't follow semver format.`,

	"ref_help": `The tag, branch or commit sha that will be tagged for the next release.`,

	"allow_first_release_help": `When there is no previous version to be found, return 0.1.0 instead of erroring out.`,

	"create_tag_help": `Create a tag for the new release.`,

	"version_help": `output semver-next's version and exit`,

	"max_bump_help": `The maximum amount to bump the version. Valid values are MAJOR, MINOR and PATCH`,

	"max_bump_enum": `MAJOR,MINOR,PATCH`,

	"min_bump_help": `The maximum amount to bump the version. Valid values are MAJOR, MINOR, PATCH and NONE. This will
be ignored when there are no commits between the previous release and the target ref.`,

	"min_bump_enum": `MAJOR,MINOR,PATCH,NONE`,

	"require_labels_help": `Require labels on pull requests when commits come from PRs`,

	"require_change_help": `Exit code is 10 if there have been no version changes since the last tag`,
}

var mainHelp = `
semver-next will analyze the merged pull requests and commits since a GitHub repository's 
latest release to determine the next release version based on semantic version rules.
`

type cmd struct {
	Repo                   string      `kong:"arg,required,help=${repo_help}"`
	Ref                    string      `kong:"short=r,default=master,help=${ref_help}"`
	PreviousReleaseVersion string      `kong:"short=v,placeholder=VERSION,help=${prev_version_help}"`
	PreviousReleaseTag     string      `kong:"placeholder=TAG,help=${prev_tag_help}"`
	MaxBump                string      `kong:"enum=${max_bump_enum},help=${max_bump_help},default=MAJOR"`
	MinBump                string      `kong:"enum=${min_bump_enum},help=${max_bump_help},default=PATCH"`
	GithubToken            string      `kong:"required,hidden,env=GITHUB_TOKEN"`
	CreateTag              bool        `kong:"help=${create_tag_help}"`
	AllowFirstRelease      bool        `kong:"help=${allow_first_release_help}"`
	Version                versionFlag `kong:"help=${version_help}"`
	RequireLabels          bool        `kong:"help=${require_labels_help}"`
	RequireChange          bool        `kong:"help=${require_change_help}"`
}

func (c *cmd) repoName() string {
	return strings.Split(c.Repo, "/")[1]
}

func (c *cmd) repoOwner() string {
	return strings.Split(c.Repo, "/")[0]
}

type versionFlag bool

func (d versionFlag) BeforeApply(k *kong.Context) error {
	k.Printf("version %s", version)
	k.Kong.Exit(0)
	return nil
}

var changeLevels = map[string]changeLevel{
	"MAJOR": changeLevelMajor,
	"MINOR": changeLevelMinor,
	"PATCH": changeLevelPatch,
	"NONE":  changeLevelNoChange,
}

func main() {
	ctx := context.Background()
	var cli cmd
	parser := kong.Must(&cli, kongVars, kong.Description(mainHelp))
	k, err := parser.Parse(os.Args[1:])
	parser.FatalIfErrorf(err)
	repoParts := strings.Split(cli.Repo, "/")
	if len(repoParts) != 2 {
		panic("Repo must be in the form of owner/repo")
	}

	client := github.NewClient(
		oauth2.NewClient(
			ctx,
			oauth2.StaticTokenSource(&oauth2.Token{AccessToken: cli.GithubToken})),
	)

	newVersion, lastReleaseVersion, firstRelease, err := getSemver(ctx, client, &cli)
	k.FatalIfErrorf(err)

	fmt.Println(newVersion)
	if !firstRelease && cli.RequireChange && newVersion.Equal(lastReleaseVersion) {
		parser.Exit(10)
	}

	if cli.CreateTag {
		tag := fmt.Sprintf("v%s", newVersion)
		err = createTag(ctx, client.Repositories, client.Git, cli.repoOwner(), cli.repoName(), tag, cli.Ref)
		k.FatalIfErrorf(err, "could not create tag")
	}
}

func getSemver(ctx context.Context, ghClient *github.Client, cli *cmd) (version, latestRelease *semver.Version, firstRelease bool, _ error) {
	var lastReleaseVersion *semver.Version
	var err error
	if cli.PreviousReleaseVersion != "" {
		lastReleaseVersion, err = semver.NewVersion(cli.PreviousReleaseVersion)
		if err != nil {
			return nil, nil, false, fmt.Errorf("last-release-version must be a valid semver")
		}
	}

	lastTag := cli.PreviousReleaseTag
	var lastReleaseName string

	if lastTag == "" {
		var lr *release
		lr, err = getLatestRelease(ctx, ghClient.Repositories, cli.repoOwner(), cli.repoName())
		if err != nil {
			return nil, nil, false, fmt.Errorf("could not get latest tag: %v", err)
		}
		if lr != nil {
			lastReleaseName = lr.name
			lastTag = lr.tag
		}
	}

	if lastTag == "" {
		if cli.AllowFirstRelease {
			return semver.New(0, 1, 0, "", ""), nil, true, nil
		}
		return nil, nil, false, fmt.Errorf("could not find a previous tag and allow-first-release is not set")
	}

	if lastReleaseVersion == nil {
		lastReleaseVersion, err = calcLastReleaseVersion(lastTag, lastReleaseName)
		if err != nil || lastReleaseVersion == nil {
			return nil, nil, false, fmt.Errorf("could not calculate previous release version and allow-first-release is not set")
		}
	}

	commits, err := diffCommits(ctx, ghClient.Repositories, ghClient.PullRequests, lastTag, cli.Ref, cli.repoOwner(), cli.repoName(), nil)
	if err != nil {
		return nil, nil, false, err
	}

	unlabeled := getUnlabeledCommits(commits)
	var unlabeledMsg []string
	for _, c := range unlabeled {
		if len(c.pulls) == 0 {
			continue
		}
		msgLine := fmt.Sprintf("%s: ", c.sha)
		for _, p := range c.pulls {
			msgLine += fmt.Sprintf("#%d ", p.number)
		}
		unlabeledMsg = append(unlabeledMsg, msgLine)
	}

	if len(unlabeledMsg) > 0 && cli.RequireLabels {
		return nil, nil, false, fmt.Errorf("some commits do not have a PR label\n%s", strings.Join(unlabeledMsg, "\n"))
	}

	newVersion := nextVersion(*lastReleaseVersion, commits, changeLevels[cli.MinBump], changeLevels[cli.MaxBump])
	return &newVersion, lastReleaseVersion, false, nil
}

func calcLastReleaseVersion(lastTag, lastReleaseName string) (*semver.Version, error) {
	ver, err := semver.NewVersion(lastTag)
	if err == nil {
		return ver, nil
	}
	ver, err = semver.NewVersion(lastReleaseName)
	if err == nil {
		return ver, nil
	}
	return nil, fmt.Errorf("no version to return")
}
