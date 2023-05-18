package main

import (
	"context"

	"github.com/google/go-github/v52/github"
)

type wrapper interface {
	ListPullRequestsWithCommit(ctx context.Context, owner, repo, sha string) ([]ResultPull, error)
	CompareCommits(ctx context.Context, owner, repo, base, head string) ([]string, error)
}

type ghWrapper struct {
	client *github.Client
}

func (g *ghWrapper) ListPullRequestsWithCommit(ctx context.Context, owner, repo, sha string) ([]ResultPull, error) {
	var result []ResultPull
	opts := &github.PullRequestListOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}
	for {
		apiPulls, resp, err := g.client.PullRequests.ListPullRequestsWithCommit(ctx, owner, repo, sha, opts)
		if err != nil {
			return nil, err
		}
		for _, apiPull := range apiPulls {
			if apiPull.GetMergedAt().IsZero() {
				continue
			}
			resultPull := ResultPull{
				Number: apiPull.GetNumber(),
				Labels: make([]string, len(apiPull.Labels)),
			}
			for i, label := range apiPull.Labels {
				resultPull.Labels[i] = label.GetName()
			}
			result = append(result, resultPull)
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return result, nil
}

func (g *ghWrapper) CompareCommits(ctx context.Context, owner, repo, base, head string) ([]string, error) {
	var result []string
	opts := &github.ListOptions{PerPage: 100}
	for {
		comp, resp, err := g.client.Repositories.CompareCommits(ctx, owner, repo, base, head, opts)
		if err != nil {
			return nil, err
		}
		for _, commit := range comp.Commits {
			result = append(result, commit.GetSHA())
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return result, nil
}
