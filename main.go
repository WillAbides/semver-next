package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/WillAbides/semver-next/internal"
	"github.com/alecthomas/kong"
	"github.com/google/go-github/v28/github"
	"golang.org/x/oauth2"
)

var kongVars = kong.Vars{
	"repo_help": `GitHub repository in "<owner>/<repo>" format. e.g. WillAbides/semver-next`,

	"prev_tag_help": `The git tag from the previous release. This should rarely be needed. When this is unset, it uses 
the tag of the release marked "latest release" on the GitHub releases page.`,

	"prev_version_help": `The version of the previous release in semver format. This may be necessary when release tags 
don't follow semver format.`,

	"ref_help": `The tag, branch or commit sha that will be tagged for the next release.`,

	"allow_first_release_help": `When there is no previous version to be found, return 0.1.0 instead of erroring out.`,

	"create_tag_help": `Create a tag for the new release.`,
}

var mainHelp = `
semver-next will analyze the merged pull requests and commits since a GitHub repository's 
latest release to determine the next release version based on semantic version rules.
`

var cli struct {
	Repo                   string `kong:"arg,required,help=${repo_help}"`
	Ref                    string `kong:"short=r,default=master,help=${ref_help}"`
	PreviousReleaseVersion string `kong:"short=v,placeholder=VERSION,help=${prev_version_help}"`
	PreviousReleaseTag     string `kong:"placeholder=TAG,help=${prev_tag_help}"`
	GithubToken            string `kong:"required,hidden,env=GITHUB_TOKEN"`
	CreateTag              bool   `kong:"help=${create_tag_help}"`
	AllowFirstRelease      bool   `kong:"help=${allow_first_release_help}"`
}

func main() {
	parser := kong.Must(
		&cli,
		kongVars,
		kong.Description(mainHelp),
	)
	_, err := parser.Parse(os.Args[1:])
	parser.FatalIfErrorf(err)
	repoParts := strings.Split(cli.Repo, "/")
	if len(repoParts) != 2 {
		panic("Repo must be in the form of owner/repo")
	}
	owner := repoParts[0]
	repo := repoParts[1]

	var lastReleaseVersion *semver.Version
	if cli.PreviousReleaseVersion != "" {
		lastReleaseVersion, err = semver.NewVersion(cli.PreviousReleaseVersion)
		if err != nil {
			log.Fatal("last-release-version must be a valid semver")
		}
	}

	ctx := context.Background()

	client := internal.WrapClient(
		github.NewClient(
			oauth2.NewClient(
				ctx,
				oauth2.StaticTokenSource(&oauth2.Token{AccessToken: cli.GithubToken})),
		),
	)

	lastTag := cli.PreviousReleaseTag
	var lastReleaseName string

	if lastTag == "" {
		var lr *internal.Release
		lr, err = internal.LatestRelease(ctx, client, owner, repo)
		if err != nil {
			log.Fatalf("could not get latest tag: %v", err)
		}
		if lr != nil {
			lastReleaseName = lr.Name
			lastTag = lr.Tag
		}
	}

	if lastTag == "" {
		if cli.AllowFirstRelease {
			fmt.Println("0.1.0")
			return
		}
		log.Fatal("could not find a previous tag and allow-first-release is not set.")
	}

	if lastReleaseVersion == nil {
		lastReleaseVersion, err = calcLastReleaseVersion(lastTag, lastReleaseName)
		if err != nil || lastReleaseVersion == nil {
			log.Fatal("could not calculate previous release version and allow-first-release is not set")
		}
	}

	commits, err := internal.DiffCommits(ctx, client, lastTag, cli.Ref, owner, repo, nil)
	if err != nil {
		panic(err)
	}

	newVersion := internal.NextVersion(*lastReleaseVersion, commits)
	fmt.Println(newVersion)

	if cli.CreateTag {
		err = internal.CreateTag(ctx, client, owner, repo, fmt.Sprintf("v%s", newVersion), cli.Ref)
		if err != nil {
			log.Fatal("could not create tag.")
		}
	}
}

func calcLastReleaseVersion(lastTag string, lastReleaseName string) (*semver.Version, error) {
	version, err := semver.NewVersion(lastTag)
	if err == nil {
		return version, nil
	}
	version, err = semver.NewVersion(lastReleaseName)
	if err == nil {
		return version, nil
	}
	return nil, fmt.Errorf("no version to return")
}
