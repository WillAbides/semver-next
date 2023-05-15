package main

import (
	"context"

	"github.com/google/go-github/v52/github"
)

type GithubPullRequestsService interface {
	ListPullRequestsWithCommit(ctx context.Context, owner, repo, sha string, opt *github.PullRequestListOptions) ([]*github.PullRequest, *github.Response, error)
}

type GithubRepositoriesService interface {
	ListCommits(ctx context.Context, owner, repo string, opt *github.CommitsListOptions) ([]*github.RepositoryCommit, *github.Response, error)
	GetCommitSHA1(ctx context.Context, owner, repo, ref, lastSHA string) (string, *github.Response, error)
	GetLatestRelease(ctx context.Context, owner, repo string) (*github.RepositoryRelease, *github.Response, error)
}

type GithubGitService interface {
	CreateRef(ctx context.Context, owner, repo string, ref *github.Reference) (*github.Reference, *github.Response, error)
	CreateTag(ctx context.Context, owner, repo string, tag *github.Tag) (*github.Tag, *github.Response, error)
}
