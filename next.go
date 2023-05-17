package main

import (
	"context"
	"github.com/google/go-github/v52/github"
)

type ResultCommit struct {
	Sha       string `json:"sha"`
	Message   string `json:"message,omitempty"`
	Merge     bool   `json:"merge,omitempty"`
	PRNumbers []int  `json:"pr_numbers,omitempty"`
}

type ResultPull struct {
	Number  int            `json:"number"`
	Labels  []string       `json:"labels,omitempty"`
	Commits []ResultCommit `json:"commits,omitempty"`
}

type Result struct {
	NextVersion     string         `json:"next_version"`
	PreviousVersion string         `json:"previous_version"`
	ChangeLevel     string         `json:"change_level"`
	Pulls           []ResultPull   `json:"pulls,omitempty"`
	Commits         []ResultCommit `json:"commits,omitempty"`
}

func foo(
	ctx context.Context,
	ghClient *github.Client,
	owner, repo,
	prevRef, prevVersion,
	targetRef string,
) error {
	//ghClient.Repositories.CompareCommits()
	return nil
}
