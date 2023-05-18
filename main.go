package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/alecthomas/kong"
	"github.com/gofri/go-github-ratelimit/github_ratelimit"
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

	"version_help": `output semver-next's version and exit`,

	"max_bump_help": `The maximum amount to bump the version.`,

	"bump_enum": `major,minor,patch,none`,

	"min_bump_help": `The maximum amount to bump the version. This will
be ignored when there are no commits between the previous release and the target ref.`,
}

var mainHelp = `
semver-next will analyze the merged pull requests and commits since a GitHub repository's 
latest release to determine the next release version based on pull request labels.
`

type cmd struct {
	Repo        string      `kong:"arg,required,help=${repo_help}"`
	Ref         string      `kong:"required,short=r,help=${ref_help}"`
	PrevRef     string      `kong:"prev,required,short=p,help=${prev_tag_help}"`
	PrevVersion string      `kong:"prev-version,short=v,help=${prev_version_help}"`
	MaxBump     string      `kong:"enum=${bump_enum},help=${max_bump_help},default=major"`
	MinBump     string      `kong:"enum=${bump_enum},help=${max_bump_help},default=none"`
	GithubToken string      `kong:"required,hidden,env=GITHUB_TOKEN"`
	Version     versionFlag `kong:"help=${version_help}"`
	Json        bool        `kong:"help=Output in JSON format"`
}

type versionFlag bool

func (d versionFlag) BeforeApply(k *kong.Context) error {
	k.Printf("version %s", version)
	k.Kong.Exit(0)
	return nil
}

func main() {
	ctx := context.Background()
	var cli cmd
	parser := kong.Must(&cli, kongVars, kong.Description(mainHelp))
	k, err := parser.Parse(os.Args[1:])
	parser.FatalIfErrorf(err)
	oauthClient := oauth2.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{AccessToken: cli.GithubToken}))
	rateLimitClient, err := github_ratelimit.NewRateLimitWaiterClient(oauthClient.Transport)
	k.FatalIfErrorf(err)
	client := github.NewClient(rateLimitClient)
	res, err := next(
		ctx,
		nextOptions{
			repo:        cli.Repo,
			gh:          &ghWrapper{client: client},
			prevVersion: cli.PrevVersion,
			base:        cli.PrevRef,
			head:        cli.Ref,
			minBump:     cli.MinBump,
			maxBump:     cli.MaxBump,
		},
	)
	k.FatalIfErrorf(err)
	if !cli.Json {
		fmt.Println(res.NextVersion)
		return
	}
	b, err := json.MarshalIndent(res, "", "  ")
	k.FatalIfErrorf(err)
	fmt.Println(string(b))
}
