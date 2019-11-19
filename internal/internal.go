package internal

import (
	"context"
	"net/http"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/google/go-github/v28/github"
)

var prefixLevels = map[string]ChangeLevel{
	"feat":            ChangeLevelMinor,
	"feature":         ChangeLevelMinor,
	"fix":             ChangeLevelPatch,
	"bugfix":          ChangeLevelPatch,
	"perf":            ChangeLevelPatch,
	"breaking":        ChangeLevelMajor,
	"breaking change": ChangeLevelMajor,
	"security":        ChangeLevelPatch,
	"patch":           ChangeLevelPatch,
}

var labelLevels = map[string]ChangeLevel{
	"breaking":        ChangeLevelMajor,
	"breaking change": ChangeLevelMajor,
	"bug":             ChangeLevelPatch,
	"enhancement":     ChangeLevelMinor,
}

type commit struct {
	message string
	pulls   []pull
}

type Release struct {
	Name string
	Tag  string
}

//go:generate mockgen -source internal.go -destination ./mocks/mock_internal.go

type GithubPullRequestsService interface {
	ListPullRequestsWithCommit(ctx context.Context, owner, repo, sha string, opt *github.PullRequestListOptions) ([]*github.PullRequest, *github.Response, error)
}

type GithubRepositoriesService interface {
	CompareCommits(ctx context.Context, owner, repo string, base, head string) (*github.CommitsComparison, *github.Response, error)
	GetLatestRelease(ctx context.Context, owner, repo string) (*github.RepositoryRelease, *github.Response, error)
}

func WrapClient(client *github.Client) *ClientWrapper {
	return &ClientWrapper{
		client: client,
	}
}

type ClientWrapper struct {
	client        *github.Client
	_pullRequests GithubPullRequestsService
	_repositories GithubRepositoriesService
}

func (w *ClientWrapper) repositories() GithubRepositoriesService {
	if w._repositories != nil {
		return w._repositories
	}
	if w.client != nil {
		return w.client.Repositories
	}
	return nil
}

func (w *ClientWrapper) pullRequests() GithubPullRequestsService {
	if w._pullRequests != nil {
		return w._pullRequests
	}
	if w.client != nil {
		return w.client.PullRequests
	}
	return nil
}

func LatestRelease(ctx context.Context, client *ClientWrapper, owner, repo string) (*Release, error) {
	var repoRelease *github.RepositoryRelease
	var err error
	repoRelease, _, err = client.repositories().GetLatestRelease(ctx, owner, repo)
	if err != nil {
		// err should be nil when status is 404
		if errResp, ok := err.(*github.ErrorResponse); ok {
			if errResp.Response.StatusCode == http.StatusNotFound {
				err = nil
			}
		}
		return nil, err
	}
	return &Release{
		Name: repoRelease.GetName(),
		Tag:  repoRelease.GetTagName(),
	}, nil
}

type commitBuilder func(ctx context.Context, client *ClientWrapper, owner string, repo string, repoCommit github.RepositoryCommit) (commit, error)

func DiffCommits(ctx context.Context, client *ClientWrapper, oldTag, newRef, owner, repo string, bc commitBuilder) ([]commit, error) {
	if bc == nil {
		bc = buildCommit
	}
	comp, _, err := client.repositories().CompareCommits(ctx, owner, repo, oldTag, newRef)
	if err != nil {
		return nil, err
	}
	if comp == nil {
		return []commit{}, nil
	}
	commits := make([]commit, len(comp.Commits))
	for commitIt, ghCommit := range comp.Commits {
		commits[commitIt], err = bc(ctx, client, owner, repo, ghCommit)
		if err != nil {
			return nil, err
		}
	}
	return commits, nil
}

func buildCommit(ctx context.Context, client *ClientWrapper, owner string, repo string, repoCommit github.RepositoryCommit) (commit, error) {
	commitPulls, _, err := client.pullRequests().ListPullRequestsWithCommit(ctx, owner, repo, repoCommit.GetSHA(), &github.PullRequestListOptions{
		State: "merged",
	})
	if err != nil {
		return commit{}, err
	}
	pls := make([]pull, len(commitPulls))
	for pullIt, pl := range commitPulls {
		lbls := make([]string, len(pl.Labels))
		for i, label := range pl.Labels {
			lbls[i] = label.GetName()
		}
		pls[pullIt] = pull{
			number: pl.GetNumber(),
			labels: lbls,
		}
	}
	c := commit{
		message: repoCommit.GetCommit().GetMessage(),
		pulls:   pls,
	}
	return c, nil
}

func NextVersion(version semver.Version, commits []commit) semver.Version {
	level := ChangeLevelNoChange
	for _, commit := range commits {
		level = level.Greater(commit.level())
	}
	switch level {
	case ChangeLevelPatch:
		return version.IncPatch()
	case ChangeLevelMinor:
		return version.IncMinor()
	case ChangeLevelMajor:
		return version.IncMajor()
	default:
		return version.IncPatch()
	}
}

func (c commit) level() ChangeLevel {
	level := parseCommitMessage(c.message)
	for _, p := range c.pulls {
		level = level.Greater(p.level())
	}
	return level
}

func parseCommitMessage(message string) ChangeLevel {
	level := ChangeLevelNoChange
	message = strings.ReplaceAll(message, "\r\n", "\n")
	lines := strings.Split(message, "\n")
	for _, line := range lines {
		if !strings.ContainsRune(line, ':') {
			continue
		}
		prefix := strings.Split(line, ":")[0]
		prefix = strings.TrimSpace(prefix)
		prefix = strings.ToLower(prefix)
		prefixLevel, ok := prefixLevels[prefix]
		if !ok {
			continue
		}
		level = level.Greater(prefixLevel)
	}
	return level
}

type pull struct {
	number int
	labels []string
}

func (p pull) level() ChangeLevel {
	level := ChangeLevelNoChange
	for _, label := range p.labels {
		label = strings.ToLower(strings.TrimSpace(label))
		labelLevel, ok := labelLevels[label]
		if !ok {
			continue
		}
		level = level.Greater(labelLevel)
	}
	return level
}

type ChangeLevel int

const (
	ChangeLevelNoChange ChangeLevel = iota
	ChangeLevelPatch
	ChangeLevelMinor
	ChangeLevelMajor
)

//Greater returns whichever is higher, c or other
func (c ChangeLevel) Greater(other ChangeLevel) ChangeLevel {
	if other > c {
		return other
	}
	return c
}
