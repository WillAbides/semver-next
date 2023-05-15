package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/google/go-github/v52/github"
)

var prefixLevels = map[string]changeLevel{
	"feat":            changeLevelMinor,
	"feature":         changeLevelMinor,
	"fix":             changeLevelPatch,
	"bugfix":          changeLevelPatch,
	"perf":            changeLevelPatch,
	"breaking":        changeLevelMajor,
	"breaking change": changeLevelMajor,
	"security":        changeLevelPatch,
	"patch":           changeLevelPatch,
}

var labelLevels = map[string]changeLevel{
	"breaking":        changeLevelMajor,
	"breaking change": changeLevelMajor,
	"bug":             changeLevelPatch,
	"enhancement":     changeLevelMinor,
	"patch":           changeLevelPatch,
}

type commit struct {
	sha     string
	message string
	pulls   []pull
}

func (c commit) level() changeLevel {
	level := parseCommitMessage(c.message)
	for _, p := range c.pulls {
		level = level.greater(pullLevel(p.labels))
	}
	return level
}

type release struct {
	name string
	tag  string
}

func createTag(
	ctx context.Context,
	reposClient GithubRepositoriesService,
	gitClient GithubGitService,
	owner, repo, tag, targetRef string,
) error {
	targetSha, _, err := reposClient.GetCommitSHA1(ctx, owner, repo, targetRef, "")
	if err != nil {
		return err
	}
	tagObj, _, err := gitClient.CreateTag(ctx, owner, repo, &github.Tag{
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
	_, _, err = gitClient.CreateRef(ctx, owner, repo, &github.Reference{
		Ref: github.String(tagRef),
		Object: &github.GitObject{
			SHA: tagObj.SHA,
		},
	})
	return err
}

func getLatestRelease(ctx context.Context, repoClient GithubRepositoriesService, owner, repo string) (*release, error) {
	var repoRelease *github.RepositoryRelease
	var err error
	repoRelease, _, err = repoClient.GetLatestRelease(ctx, owner, repo)
	if err != nil {
		// err should be nil when status is 404
		if errResp, ok := err.(*github.ErrorResponse); ok {
			if errResp.Response.StatusCode == http.StatusNotFound {
				err = nil
			}
		}
		return nil, err
	}
	return &release{
		name: repoRelease.GetName(),
		tag:  repoRelease.GetTagName(),
	}, nil
}

type commitBuilder func(ctx context.Context, prClient GithubPullRequestsService, owner, repo string, repoCommit github.RepositoryCommit) (commit, error)

func diffCommits(
	ctx context.Context,
	reposClient GithubRepositoriesService,
	prClient GithubPullRequestsService,
	oldTag, newRef, owner, repo string,
	bc commitBuilder,
) ([]commit, error) {
	if bc == nil {
		bc = buildCommit
	}

	oldSha1, _, err := reposClient.GetCommitSHA1(ctx, owner, repo, oldTag, "")
	if err != nil {
		return nil, err
	}

	opt := &github.CommitsListOptions{
		SHA: newRef,
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	var commits []commit
	for {
		repoCommits, resp, err := reposClient.ListCommits(ctx, owner, repo, opt)
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
			c, err := bc(ctx, prClient, owner, repo, *repoCommit)
			if err != nil {
				return nil, err
			}
			commits = append(commits, c)
		}
		if resp.NextPage == 0 || hitLastSha {
			break
		}
		opt.Page = resp.NextPage
	}
	return commits, nil
}

func buildCommit(ctx context.Context, prClient GithubPullRequestsService, owner, repo string, repoCommit github.RepositoryCommit) (commit, error) {
	commitPulls, _, err := prClient.ListPullRequestsWithCommit(ctx, owner, repo, repoCommit.GetSHA(), &github.PullRequestListOptions{
		State: "merged",
	})
	if err != nil {
		return commit{}, err
	}
	pls := make([]pull, len(commitPulls))
	for i, pl := range commitPulls {
		lbls := make([]string, len(pl.Labels))
		for i, label := range pl.Labels {
			lbls[i] = label.GetName()
		}
		pls[i] = pull{
			number: pl.GetNumber(),
			labels: lbls,
		}
	}
	return commit{
		sha:     repoCommit.GetSHA(),
		message: repoCommit.GetCommit().GetMessage(),
		pulls:   pls,
	}, nil
}

func nextVersion(version semver.Version, commits []commit, minBump, maxBump changeLevel) semver.Version {
	if len(commits) == 0 {
		return version
	}
	level := changeLevelNoChange
	for _, c := range commits {
		level = level.greater(c.level())
	}
	level = level.greater(minBump)
	level = level.lesser(maxBump)
	switch level {
	case changeLevelPatch:
		return version.IncPatch()
	case changeLevelMinor:
		return version.IncMinor()
	case changeLevelMajor:
		return version.IncMajor()
	case changeLevelNoChange:
		return version
	}
	return version
}

func getUnlabeledCommits(commits []commit) []commit {
	result := make([]commit, 0, len(commits))
	for _, c := range commits {
		if len(c.pulls) == 0 {
			continue
		}
		labeled := false
		for _, p := range c.pulls {
			for _, label := range p.labels {
				if _, ok := labelLevels[label]; ok {
					labeled = true
					break
				}
			}
			if labeled {
				break
			}
		}
		if !labeled {
			result = append(result, c)
		}
	}
	return result
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

func parseCommitMessage(message string) changeLevel {
	level := changeLevelNoChange
	prefixes := commitMessagePrefixes(message)
	for _, prefix := range prefixes {
		level = level.greater(prefixLevels[prefix])
	}
	return level
}

type pull struct {
	number int
	labels []string
}

func pullLevel(labels []string) changeLevel {
	level := changeLevelNoChange
	for _, label := range labels {
		label = strings.ToLower(strings.TrimSpace(label))
		labelLevel, ok := labelLevels[label]
		if !ok {
			continue
		}
		level = level.greater(labelLevel)
	}
	return level
}

type changeLevel int

const (
	changeLevelNoChange changeLevel = iota
	changeLevelPatch
	changeLevelMinor
	changeLevelMajor
)

// lesser returns whichever is lower, c or other
func (c changeLevel) lesser(other changeLevel) changeLevel {
	if other < c {
		return other
	}
	return c
}

// greater returns whichever is higher, c or other
func (c changeLevel) greater(other changeLevel) changeLevel {
	if other > c {
		return other
	}
	return c
}
