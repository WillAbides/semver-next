package main

import (
	"context"
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/google/go-github/v52/github"
	"github.com/stretchr/testify/assert"
)

type prClientStub struct {
	listPullRequestsWithCommit func(ctx context.Context, owner, repo, sha string, opt *github.PullRequestListOptions) ([]*github.PullRequest, *github.Response, error)
}

func (s *prClientStub) ListPullRequestsWithCommit(ctx context.Context, owner, repo, sha string, opt *github.PullRequestListOptions) ([]*github.PullRequest, *github.Response, error) {
	return s.listPullRequestsWithCommit(ctx, owner, repo, sha, opt)
}

type gitClientStub struct {
	createRef func(ctx context.Context, owner, repo string, ref *github.Reference) (*github.Reference, *github.Response, error)
	createTag func(ctx context.Context, owner, repo string, tag *github.Tag) (*github.Tag, *github.Response, error)
}

func (s *gitClientStub) CreateRef(ctx context.Context, owner, repo string, ref *github.Reference) (*github.Reference, *github.Response, error) {
	return s.createRef(ctx, owner, repo, ref)
}

func (s *gitClientStub) CreateTag(ctx context.Context, owner, repo string, tag *github.Tag) (*github.Tag, *github.Response, error) {
	return s.createTag(ctx, owner, repo, tag)
}

type reposClientStub struct {
	listCommits      func(ctx context.Context, owner, repo string, opt *github.CommitsListOptions) ([]*github.RepositoryCommit, *github.Response, error)
	getCommitSHA1    func(ctx context.Context, owner, repo, ref, lastSHA string) (string, *github.Response, error)
	getLatestRelease func(ctx context.Context, owner, repo string) (*github.RepositoryRelease, *github.Response, error)
}

func (s *reposClientStub) ListCommits(ctx context.Context, owner, repo string, opt *github.CommitsListOptions) ([]*github.RepositoryCommit, *github.Response, error) {
	return s.listCommits(ctx, owner, repo, opt)
}

func (s *reposClientStub) GetCommitSHA1(ctx context.Context, owner, repo, ref, lastSHA string) (string, *github.Response, error) {
	return s.getCommitSHA1(ctx, owner, repo, ref, lastSHA)
}

func (s *reposClientStub) GetLatestRelease(ctx context.Context, owner, repo string) (*github.RepositoryRelease, *github.Response, error) {
	return s.getLatestRelease(ctx, owner, repo)
}

func TestCreateTag(t *testing.T) {
	ctx := context.Background()
	newTag := "newtag"
	commitSha := "4ee551021fc59c2d45c2d7a91b1562914e4dff61"
	tagObjSha := "7065ecdd3f84fc92fe8b7d3fb3927a0974f5dc37"
	mockReposSvc := &reposClientStub{
		getCommitSHA1: func(ctx context.Context, owner, repo, ref, lastSHA string) (string, *github.Response, error) {
			assert.Equal(t, "foo", owner)
			assert.Equal(t, "bar", repo)
			assert.Equal(t, "deadbeef", ref)
			assert.Equal(t, "", lastSHA)
			return commitSha, nil, nil
		},
	}
	mockGitSvc := &gitClientStub{
		createRef: func(ctx context.Context, owner, repo string, ref *github.Reference) (*github.Reference, *github.Response, error) {
			assert.Equal(t, "foo", owner)
			assert.Equal(t, "bar", repo)
			assert.Equal(t, "refs/tags/newtag", ref.GetRef())
			assert.Equal(t, tagObjSha, ref.GetObject().GetSHA())
			return nil, nil, nil
		},
		createTag: func(ctx context.Context, owner, repo string, tag *github.Tag) (*github.Tag, *github.Response, error) {
			assert.Equal(t, "foo", owner)
			assert.Equal(t, "bar", repo)
			assert.Equal(t, newTag, tag.GetTag())
			assert.Equal(t, newTag, tag.GetMessage())
			assert.Equal(t, "commit", tag.GetObject().GetType())
			assert.Equal(t, commitSha, tag.GetObject().GetSHA())
			return &github.Tag{SHA: &tagObjSha}, nil, nil
		},
	}
	err := createTag(ctx, mockReposSvc, mockGitSvc, "foo", "bar", "newtag", "deadbeef")
	assert.NoError(t, err)
}

func TestLatestRelease(t *testing.T) {
	ctx := context.Background()
	wantReleaseName := "want release name"
	wantTagName := "want tag name"
	mockReposSvc := &reposClientStub{
		getLatestRelease: func(ctx context.Context, owner, repo string) (*github.RepositoryRelease, *github.Response, error) {
			assert.Equal(t, "foo", owner)
			assert.Equal(t, "bar", repo)
			return &github.RepositoryRelease{
					Name:    github.String(wantReleaseName),
					TagName: github.String(wantTagName),
				},
				&github.Response{},
				nil
		},
	}

	got, err := getLatestRelease(ctx, mockReposSvc, "foo", "bar")
	assert.NoError(t, err)
	assert.Equal(t, wantReleaseName, got.name)
	assert.Equal(t, wantTagName, got.tag)
}

func Test_buildCommit(t *testing.T) {
	ctx := context.Background()
	repoCommit := github.RepositoryCommit{
		Commit: &github.Commit{
			Message: github.String("commit message"),
		},
		SHA: github.String("deadbeef"),
	}
	mockPullsSvc := &prClientStub{
		listPullRequestsWithCommit: func(ctx context.Context, owner, repo, sha string, opt *github.PullRequestListOptions) ([]*github.PullRequest, *github.Response, error) {
			assert.Equal(t, "foo", owner)
			assert.Equal(t, "bar", repo)
			assert.Equal(t, "deadbeef", sha)
			assert.Equal(t, "merged", opt.State)
			return []*github.PullRequest{
				{
					Number: github.Int(12),
					Labels: []*github.Label{
						{Name: github.String("label 1")},
						{Name: github.String("label 2")},
					},
				},
			}, &github.Response{}, nil
		},
	}
	want := commit{
		sha:     "deadbeef",
		message: "commit message",
		pulls: []pull{
			{
				number: 12,
				labels: []string{"label 1", "label 2"},
			},
		},
	}
	got, err := buildCommit(ctx, mockPullsSvc, "foo", "bar", repoCommit)
	assert.NoError(t, err)
	assert.Equal(t, want, got)
}

func Test_diffCommits(t *testing.T) {
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
	wantCommit := commit{
		message: "foo",
		pulls: []pull{
			{
				number: 12,
				labels: []string{"label 1", "label 2"},
			},
		},
	}
	bc := func(_ context.Context, _ GithubPullRequestsService, owner, repo string, repoCommit github.RepositoryCommit) (commit, error) {
		t.Helper()
		assert.Equal(t, exOwner, owner)
		assert.Equal(t, exRepo, repo)
		assert.Contains(t, repoCommits, &repoCommit)
		return wantCommit, nil
	}

	ctx := context.Background()
	mockReposSvc := &reposClientStub{
		getCommitSHA1: func(ctx context.Context, owner, repo, ref, path string) (string, *github.Response, error) {
			assert.Equal(t, exOwner, owner)
			assert.Equal(t, exRepo, repo)
			assert.Equal(t, base, ref)
			assert.Equal(t, "", path)
			return "oldbeef", &github.Response{}, nil
		},
		listCommits: func(ctx context.Context, owner, repo string, opt *github.CommitsListOptions) ([]*github.RepositoryCommit, *github.Response, error) {
			assert.Equal(t, exOwner, owner)
			assert.Equal(t, exRepo, repo)
			assert.Equal(t, head, opt.SHA)
			assert.Equal(t, 100, opt.ListOptions.PerPage)
			return repoCommits, &github.Response{NextPage: 1}, nil
		},
	}

	got, err := diffCommits(ctx, mockReposSvc, nil, base, head, exOwner, exRepo, bc)
	assert.NoError(t, err)
	assert.Equal(t, []commit{wantCommit}, got)
}

func Test_parseCommitMessage(t *testing.T) {
	for _, td := range []struct {
		message string
		want    changeLevel
	}{
		{message: `feat: omg`, want: changeLevelMinor},
		{message: `omg`, want: changeLevelNoChange},
		{message: ``, want: changeLevelNoChange},
		{message: `breaking: omg`, want: changeLevelMajor},
		{
			message: `
foo: bar
breaking: omg

`,
			want: changeLevelMajor,
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
	got := nextVersion(*ver, commits, changeLevelNoChange, changeLevelMajor)
	assert.Equal(t, "2.0.0", got.String())
}
