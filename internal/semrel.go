package internal

import (
	"context"
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

func DiffCommits(ctx context.Context, client *github.Client, oldTag, newRef, owner, repo string) ([]commit, error ){
	comp, _, err := client.Repositories.CompareCommits(ctx, owner, repo, oldTag, newRef)
	if err != nil {
		return nil, err
	}
	commits := make([]commit, len(comp.Commits))
	for commitIt, cmmt := range comp.Commits {
		commitPulls, _, err := client.PullRequests.ListPullRequestsWithCommit(ctx, "WillAbides", "bindownloader", cmmt.GetSHA(), &github.PullRequestListOptions{
			State: "merged",
		})
		if err != nil {
			return nil, err
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
		commits[commitIt] = commit{
			message: cmmt.GetCommit().GetMessage(),
			pulls:   pls,
		}
	}
	return commits, nil
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
