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

func TestCreateTag(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockGitSvc := mocks.NewMockGithubGitService(ctrl)
	mockReposSvc := mocks.NewMockGithubRepositoriesService(ctrl)
	wrapper := &ClientWrapper{
		_git:          mockGitSvc,
		_repositories: mockReposSvc,
	}
	newTag := "newtag"
	commitSha := "4ee551021fc59c2d45c2d7a91b1562914e4dff61"
	tagObjSha := "7065ecdd3f84fc92fe8b7d3fb3927a0974f5dc37"
	mockReposSvc.EXPECT().GetCommitSHA1(ctx, "foo", "bar", "deadbeef", "").Return(commitSha, nil, nil)
	mockGitSvc.EXPECT().CreateTag(ctx, "foo", "bar", &github.Tag{
		Tag:     &newTag,
		Message: &newTag,
		Object: &github.GitObject{
			Type: github.String("commit"),
			SHA:  &commitSha,
		},
	}).Return(&github.Tag{SHA: &tagObjSha}, nil, nil)
	mockGitSvc.EXPECT().CreateRef(ctx, "foo", "bar", &github.Reference{
		Ref: github.String("refs/tags/newtag"),
		Object: &github.GitObject{
			SHA: &tagObjSha,
		},
	})
	err := CreateTag(ctx, wrapper, "foo", "bar", "newtag", "deadbeef")
	assert.NoError(t, err)
}

func TestLatestRelease(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockReposSvc := mocks.NewMockGithubRepositoriesService(ctrl)
	wrapper := &ClientWrapper{
		_repositories: mockReposSvc,
	}
	ctx := context.Background()
	wantReleaseName := "want release name"
	wantTagName := "want tag name"
	mockReposSvc.EXPECT().GetLatestRelease(ctx, "foo", "bar").Return(
		&github.RepositoryRelease{
			Name:    github.String(wantReleaseName),
			TagName: github.String(wantTagName),
		},
		&github.Response{},
		nil,
	)

	got, err := LatestRelease(ctx, wrapper, "foo", "bar")
	assert.NoError(t, err)
	assert.Equal(t, wantReleaseName, got.Name)
	assert.Equal(t, wantTagName, got.Tag)
}

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
	want := Commit{
		Sha:     "deadbeef",
		Message: "commit message",
		Pulls: []pull{
			{
				Number: 12,
				Labels: []string{"label 1", "label 2"},
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
	repoCommits := []*github.RepositoryCommit{
		{
			Commit: &github.Commit{
				Message: github.String("commit message"),
			},
			SHA: github.String("deadbeef"),
		},
		{
			Commit: &github.Commit{
				Message: github.String("commit message 2"),
			},
			SHA: github.String("oldbeef"),
		},
	}
	wantCommit := Commit{
		Message: "foo",
		Pulls: []pull{
			{
				Number: 12,
				Labels: []string{"label 1", "label 2"},
			},
		},
	}
	bc := func(_ context.Context, _ *ClientWrapper, owner string, repo string, repoCommit github.RepositoryCommit) (Commit, error) {
		t.Helper()
		assert.Equal(t, exOwner, owner)
		assert.Equal(t, exRepo, repo)
		assert.Contains(t, repoCommits, &repoCommit)
		return wantCommit, nil
	}

	ctx := context.Background()
	mockReposSvc.EXPECT().GetCommitSHA1(ctx, exOwner, exRepo, base, "").Return("oldbeef", nil, nil)
	mockReposSvc.EXPECT().ListCommits(ctx, exOwner, exRepo, &github.CommitsListOptions{
		SHA: head,
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}).Return(
		repoCommits,
		&github.Response{
			NextPage: 1,
		},
		nil,
	)

	got, err := DiffCommits(ctx, wrapper, base, head, exOwner, exRepo, bc)
	assert.NoError(t, err)
	assert.Equal(t, []Commit{wantCommit}, got)
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
	commits := []Commit{
		{
			Message: "nothing",
		},
		{
			Message: "",
		},
		{},
		{
			Message: `feat: omg
this is not a breaking change: really
`,
		},
		{
			Message: `foo`,
			Pulls: []pull{
				{
					Labels: []string{"foo", "bar", "enhancement", "breaking change"},
				},
			},
		},
	}
	ver := semver.MustParse("1.2.3")
	got := NextVersion(*ver, commits, ChangeLevelNoChange, ChangeLevelMajor)
	assert.Equal(t, "2.0.0", got.String())
}
