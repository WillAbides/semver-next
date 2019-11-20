package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/WillAbides/semver-next/internal"
	"github.com/alecthomas/kong"
	"github.com/google/go-github/v28/github"
	"golang.org/x/oauth2"
)

var cli struct {
	Repo               string `kong:"arg,required,help='owner/repo'"`
	LastReleaseTag     string `kong:"short=t"`
	Ref                string `kong:"short=r,default=master"`
	LastReleaseVersion string `kong:"short=v"`
	GithubToken        string `kong:"required,hidden,env=GITHUB_TOKEN"`
	AllowFirstRelease  bool   `kong:"help='When there is no previous version to be found, return 0.1.0 instead of erroring out.'"`
	CreateTag          bool   `kong:"create the new tag on github"`
}

func main() {
	kong.Parse(&cli)
	repoParts := strings.Split(cli.Repo, "/")
	if len(repoParts) != 2 {
		panic("Repo must be in the form of owner/repo")
	}
	owner := repoParts[0]
	repo := repoParts[1]

	var lastReleaseVersion *semver.Version
	if cli.LastReleaseVersion != "" {
		var err error
		lastReleaseVersion, err = semver.NewVersion(cli.LastReleaseVersion)
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

	lastTag := cli.LastReleaseTag
	var lastReleaseName string

	if lastTag == "" {
		lr, err := internal.LatestRelease(ctx, client, owner, repo)
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
		var err error
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
