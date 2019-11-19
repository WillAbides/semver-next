package internal

import (
	"context"
	"testing"

	"github.com/Masterminds/semver/v3"
	mocks "github.com/WillAbides/semver-next/internal/mocks"
	"github.com/golang/mock/gomock"
	"github.com/google/go-github/v28/github"
	"github.com/stretchr/testify/assert"
)

func Test_buildCommit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockPullsSvc := mocks.NewMockGithubPullRequestsService(ctrl)
	wrapper := &ClientWrapper{
		_pullRequests: mockPullsSvc,
	}
	ctx := context.Background()
	repoCommit := github.RepositoryCommit{
		Commit: &github.Commit{
			Message: github.String("commit message"),
		},
		SHA: github.String("deadbeef"),
	}
	mockPullsSvc.EXPECT().ListPullRequestsWithCommit(ctx, "foo", "bar", "deadbeef", &github.PullRequestListOptions{
		State: "merged",
	}).Return(
		[]*github.PullRequest{
			{
				Number: github.Int(12),
				Labels: []*github.Label{
					{Name: github.String("label 1")},
					{Name: github.String("label 2")},
				},
			},
		}, &github.Response{}, nil,
	)
	want := commit{
		message: "commit message",
		pulls: []pull{
			{
				number: 12,
				labels: []string{"label 1", "label 2"},
			},
		},
	}
	got, err := buildCommit(ctx, wrapper, "foo", "bar", repoCommit)
	assert.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestDiffCommits(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockReposSvc := mocks.NewMockGithubRepositoriesService(ctrl)
	wrapper := &ClientWrapper{
		_repositories: mockReposSvc,
	}
	exOwner := "fooOwner"
	exRepo := "fooRepo"
	base := "oldTag"
	head := "newRef"
	repoCommits := []github.RepositoryCommit{
		{
			Commit: &github.Commit{
				Message: github.String("commit message"),
			},
			SHA: github.String("deadbeef"),
		},
	}
	wantCommit := commit{
		message: "foo",
		pulls: []pull{
			{
				number: 12,
				labels: []string{"label 1", "label 2"},
			},
		},
	}
	bc := func(_ context.Context, _ *ClientWrapper, owner string, repo string, repoCommit github.RepositoryCommit) (commit, error) {
		t.Helper()
		assert.Equal(t, exOwner, owner)
		assert.Equal(t, exRepo, repo)
		assert.Equal(t, repoCommits[0], repoCommit)
		return wantCommit, nil
	}

	ctx := context.Background()
	mockReposSvc.EXPECT().CompareCommits(ctx, exOwner, exRepo, base, head).Return(
		&github.CommitsComparison{
			Commits: repoCommits,
		},
		&github.Response{},
		nil,
	)
	got, err := DiffCommits(ctx, wrapper, base, head, exOwner, exRepo, bc)
	assert.NoError(t, err)
	assert.Equal(t, []commit{wantCommit}, got)
}

func Test_parseCommitMessage(t *testing.T) {
	for _, td := range []struct {
		message string
		want    ChangeLevel
	}{
		{message: `feat: omg`, want: ChangeLevelMinor},
		{message: `omg`, want: ChangeLevelNoChange},
		{message: ``, want: ChangeLevelNoChange},
		{message: `breaking: omg`, want: ChangeLevelMajor},
		{message: `
foo: bar
breaking: omg

`,
			want: ChangeLevelMajor,
		},
	} {
		t.Run("", func(t *testing.T) {
			got := parseCommitMessage(td.message)
			assert.Equal(t, td.want, got)
		})
	}
}

func Test_nextVersion(t *testing.T) {
	commits := []commit{
		{
			message: "nothing",
		},
		{
			message: "",
		},
		{},
		{
			message: `feat: omg
this is not a breaking change: really
`,
		},
		{
			message: `foo`,
			pulls: []pull{
				{
					labels: []string{"foo", "bar", "enhancement", "breaking change"},
				},
			},
		},
	}
	ver := semver.MustParse("1.2.3")
	got := NextVersion(*ver, commits)
	assert.Equal(t, "2.0.0", got.String())
}