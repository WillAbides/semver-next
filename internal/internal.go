package internal

import (
	"context"
	"fmt"
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
	"patch":           ChangeLevelPatch,
}

type Commit struct {
	Sha     string
	Message string
	Pulls   []pull
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
	ListCommits(ctx context.Context, owner, repo string, opt *github.CommitsListOptions) ([]*github.RepositoryCommit, *github.Response, error)
	GetCommitSHA1(ctx context.Context, owner, repo, ref, lastSHA string) (string, *github.Response, error)
	CompareCommits(ctx context.Context, owner, repo string, base, head string) (*github.CommitsComparison, *github.Response, error)
	GetLatestRelease(ctx context.Context, owner, repo string) (*github.RepositoryRelease, *github.Response, error)
}

type GithubGitService interface {
	CreateRef(ctx context.Context, owner string, repo string, ref *github.Reference) (*github.Reference, *github.Response, error)
	CreateTag(ctx context.Context, owner string, repo string, tag *github.Tag) (*github.Tag, *github.Response, error)
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
	_git          GithubGitService
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

func (w *ClientWrapper) git() GithubGitService {
	if w._git != nil {
		return w._git
	}
	if w.client != nil {
		return w.client.Git
	}
	return nil
}

func CreateTag(ctx context.Context, client *ClientWrapper, owner, repo, tag, targetRef string) error {
	targetSha, _, err := client.repositories().GetCommitSHA1(ctx, owner, repo, targetRef, "")
	if err != nil {
		return err
	}
	tagObj, _, err := client.git().CreateTag(ctx, owner, repo, &github.Tag{
		Tag:     &tag,
		Message: &tag,
		Object: &github.GitObject{
			Type: github.String("commit"),
			SHA:  &targetSha,
		},
	})
	if err != nil {
		return err
	}
	tagRef := fmt.Sprintf("refs/tags/%s", tag)
	_, _, err = client.git().CreateRef(ctx, owner, repo, &github.Reference{
		Ref: github.String(tagRef),
		Object: &github.GitObject{
			SHA: tagObj.SHA,
		},
	})
	return err
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

type commitBuilder func(ctx context.Context, client *ClientWrapper, owner string, repo string, repoCommit github.RepositoryCommit) (Commit, error)

func DiffCommits(ctx context.Context, client *ClientWrapper, oldTag, newRef, owner, repo string, bc commitBuilder) ([]Commit, error) {
	if bc == nil {
		bc = buildCommit
	}

	oldSha1, _, err := client.repositories().GetCommitSHA1(ctx, owner, repo, oldTag, "")
	if err != nil {
		return nil, err
	}

	opt := &github.CommitsListOptions{
		SHA: newRef,
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	var commits []Commit
	for {
		repoCommits, resp, err := client.repositories().ListCommits(ctx, owner, repo, opt)
		if err != nil {
			return nil, err
		}
		var hitLastSha bool
		for _, repoCommit := range repoCommits {
			sha := repoCommit.GetSHA()
			if sha == oldSha1 {
				hitLastSha = true
				break
			}
			commit, err := bc(ctx, client, owner, repo, *repoCommit)
			if err != nil {
				return nil, err
			}
			commits = append(commits, commit)
		}
		if resp.NextPage == 0 || hitLastSha {
			break
		}
		opt.Page = resp.NextPage
	}
	return commits, nil
}

func buildCommit(ctx context.Context, client *ClientWrapper, owner string, repo string, repoCommit github.RepositoryCommit) (Commit, error) {
	commitPulls, _, err := client.pullRequests().ListPullRequestsWithCommit(ctx, owner, repo, repoCommit.GetSHA(), &github.PullRequestListOptions{
		State: "merged",
	})
	if err != nil {
		return Commit{}, err
	}
	pls := make([]pull, len(commitPulls))
	for pullIt, pl := range commitPulls {
		lbls := make([]string, len(pl.Labels))
		for i, label := range pl.Labels {
			lbls[i] = label.GetName()
		}
		pls[pullIt] = pull{
			Number: pl.GetNumber(),
			Labels: lbls,
		}
	}
	return Commit{
		Sha:     repoCommit.GetSHA(),
		Message: repoCommit.GetCommit().GetMessage(),
		Pulls:   pls,
	}, nil
}

func NextVersion(version semver.Version, commits []Commit, minBump, maxBump ChangeLevel) semver.Version {
	if len(commits) == 0 {
		return version
	}
	level := ChangeLevelNoChange
	for _, commit := range commits {
		level = level.Greater(commit.level())
	}
	level = level.Greater(minBump)
	level = level.Lesser(maxBump)
	switch level {
	case ChangeLevelPatch:
		return version.IncPatch()
	case ChangeLevelMinor:
		return version.IncMinor()
	case ChangeLevelMajor:
		return version.IncMajor()
	case ChangeLevelNoChange:
		return version
	}
	return version
}

func UnlabeledCommits(commits []Commit) []Commit {
	result := make([]Commit, 0, len(commits))
commitsLoop:
	for _, commit := range commits {
		if len(commit.Pulls) == 0 {
			continue commitsLoop
		}
		for _, p := range commit.Pulls {
			for _, label := range p.Labels {
				if _, ok := labelLevels[label]; ok {
					continue commitsLoop
				}
			}
		}
		result = append(result, commit)
	}
	return result
}

func (c Commit) level() ChangeLevel {
	level := parseCommitMessage(c.Message)
	for _, p := range c.Pulls {
		level = level.Greater(p.level())
	}
	return level
}

func commitMessagePrefixes(message string) []string {
	message = strings.ReplaceAll(message, "\r\n", "\n")
	lines := strings.Split(message, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if !strings.ContainsRune(line, ':') {
			continue
		}
		prefix := strings.Split(line, ":")[0]
		prefix = strings.TrimSpace(prefix)
		prefix = strings.ToLower(prefix)
		_, ok := prefixLevels[prefix]
		if ok {
			result = append(result, prefix)
		}
	}
	return result
}

func parseCommitMessage(message string) ChangeLevel {
	level := ChangeLevelNoChange
	prefixes := commitMessagePrefixes(message)
	for _, prefix := range prefixes {
		level = level.Greater(prefixLevels[prefix])
	}
	return level
}

type pull struct {
	Number int
	Labels []string
}

func (p pull) level() ChangeLevel {
	level := ChangeLevelNoChange
	for _, label := range p.Labels {
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

//Lesser returns whichever is lower, c or other
func (c ChangeLevel) Lesser(other ChangeLevel) ChangeLevel {
	if other < c {
		return other
	}
	return c
}

//Greater returns whichever is higher, c or other
func (c ChangeLevel) Greater(other ChangeLevel) ChangeLevel {
	if other > c {
		return other
	}
	return c
}
