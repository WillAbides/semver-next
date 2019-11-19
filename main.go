package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/WillAbides/semver-next/internal"
	"github.com/alecthomas/kong"
	"github.com/google/go-github/v28/github"
	"golang.org/x/oauth2"
)

var cli struct {
	Repo           string `kong:"arg,required,help='owner/repo'"`
	LastReleaseTag string `kong:"required,short=t"`
	Ref            string `kong:"short=r,default=master"`
}

func main() {
	kong.Parse(&cli)
	repoParts := strings.Split(cli.Repo, "/")
	if len(repoParts) != 2 {
		panic("Repo must be in the form of owner/repo")
	}
	owner := repoParts[0]
	repo := repoParts[1]
	ctx := context.Background()
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		panic("need GITHUB_TOKEN")
	}
	client := github.NewClient(oauth2.NewClient(ctx, oauth2.StaticTokenSource(
		&oauth2.Token{
			AccessToken: githubToken,
		})))

	oldVersion, err := semver.NewVersion(cli.LastReleaseTag)
	if err != nil {
		panic(err)
	}

	commits, err := internal.DiffCommits(ctx, internal.WrapClient(client), cli.LastReleaseTag, cli.Ref, owner, repo, nil)
	if err != nil {
		panic(err)
	}

	newVersion := internal.NextVersion(*oldVersion, commits)
	fmt.Println(newVersion)
}
