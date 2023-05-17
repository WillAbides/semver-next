package main

import (
	"context"
	"fmt"
	"github.com/go-semantic-release/commit-analyzer-cz/pkg/analyzer"
	"github.com/go-semantic-release/semantic-release/v2/pkg/semrel"
	"github.com/google/go-github/v52/github"
	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"os"
	"testing"
)

func Test_foo(t *testing.T) {
	z := &analyzer.DefaultCommitAnalyzer{}
	got := z.Analyze([]*semrel.RawCommit{
		{
			SHA:         "123",
			Annotations: map[string]string{},
			RawMessage:  "feat: foo",
		},
	})
	for _, g := range got {
		fmt.Printf("%+v\n", g)
	}
}

func Test_bar(t *testing.T) {
	ctx := context.Background()
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		t.Skip("GITHUB_TOKEN not set")
	}
	client := github.NewClient(oauth2.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{AccessToken: githubToken})))
	tags, _, err := client.Repositories.ListTags(ctx, "willabides", "semver-next", nil)
	require.NoError(t, err)
	fmt.Println(tags)
	mainHead, _, err := client.Repositories.GetCommit(ctx, "willabides", "semver-next", "main", nil)
	require.NoError(t, err)
	v05head, _, err := client.Repositories.GetCommit(ctx, "willabides", "semver-next", "v0.5.0", nil)
	require.NoError(t, err)
	fmt.Println(mainHead.GetSHA(), v05head.GetSHA())
	comp, _, err := client.Repositories.CompareCommits(ctx, "willabides", "semver-next", v05head.GetSHA(), mainHead.GetSHA(), &github.ListOptions{
		PerPage: 100,
	})
	require.NoError(t, err)
	fmt.Println(comp.GetAheadBy())
	fmt.Println(comp.GetTotalCommits())
	fmt.Println(len(comp.Commits))
	for _, rc := range comp.Commits {
		//if len(rc.Parents) > 1 {
		//	continue
		//}
		fmt.Println(rc.GetSHA(), rc.GetCommit().GetMessage())
	}
	//for _, tag := range tags {
	//	v, err := semver.NewVersion(tag.GetName())
	//	if err != nil {
	//		continue
	//	}
	//	fmt.Println(v.Original())
	//	comp, _, err := client.Repositories.CompareCommits(ctx, "willabides", "semver-next", tag.GetCommit().GetSHA(), mainHead.GetSHA(), nil)
	//	require.NoError(t, err)
	//	fmt.Println(comp.GetStatus())
	//}

}

func Test_gql(t *testing.T) {
	ctx := context.Background()
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		t.Skip("GITHUB_TOKEN not set")
	}
	client := githubv4.NewClient(oauth2.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{AccessToken: githubToken})))

	type commitNode struct {
		AssociatedPullRequests struct {
			Nodes []struct {
				Labels struct {
					Nodes []struct {
						Name githubv4.String
					}
				} `graphql:"labels(first: 50)"`
			}
		} `graphql:"associatedPullRequests(first: 10)"`
	}

	var query struct {
		Repository struct {
			Ref struct {
				Compare struct {
					AheadBy  githubv4.Int
					BehindBy githubv4.Int
					Commits  struct {
						PageInfo struct {
							EndCursor   githubv4.String
							HasNextPage githubv4.Boolean
						}
						Nodes []commitNode
					} `graphql:"commits(first: 10, after: $commitsCursor)"`
				} `graphql:"compare(headRef: \"main\")"`
			} `graphql:"ref(qualifiedName: \"v0.1.0\")"`
		} `graphql:"repository(name: $repoName, owner: $repoOwner)"`
	}
	variables := map[string]any{
		"repoName":      githubv4.String("semver-next"),
		"repoOwner":     githubv4.String("willabides"),
		"commitsCursor": (*githubv4.String)(nil),
	}
	var allCommits []commitNode
	for {
		err := client.Query(ctx, &query, variables)
		if err != nil {
			t.Fatal(err)
		}
		allCommits = append(allCommits, query.Repository.Ref.Compare.Commits.Nodes...)
		if !query.Repository.Ref.Compare.Commits.PageInfo.HasNextPage {
			break
		}
		variables["commitsCursor"] = githubv4.NewString(query.Repository.Ref.Compare.Commits.PageInfo.EndCursor)
	}
	fmt.Println(query.Repository.Ref.Compare.AheadBy)
	fmt.Println(len(allCommits))
}
