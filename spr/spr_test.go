package spr

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/ejoffe/spr/config"
	"github.com/ejoffe/spr/git"
	"github.com/ejoffe/spr/git/mockgit"
	"github.com/ejoffe/spr/github"
	"github.com/ejoffe/spr/github/githubclient/gen/genclient"
	"github.com/ejoffe/spr/github/mockclient"
	"github.com/stretchr/testify/require"
)

func makeTestObjects(t *testing.T, mergeMethod string) (
	s *stackediff, gitmock *mockgit.Mock, githubmock *mockclient.MockClient,
	input *bytes.Buffer, output *bytes.Buffer) {
	cfg := config.EmptyConfig()
	cfg.Repo.RequireChecks = true
	cfg.Repo.RequireApproval = true
	cfg.Repo.GitHubRemote = "origin"
	cfg.Repo.GitHubBranch = "master"
	cfg.Repo.MergeMethod = mergeMethod
	cfg.Repo.PrPrefix = "spr/master"
	gitmock = mockgit.NewMockGit(t)
	githubmock = mockclient.NewMockClient(t)
	githubmock.Info = &github.GitHubInfo{
		UserName:     "TestSPR",
		RepositoryID: "RepoID",
		LocalBranch:  "master",
	}
	s = NewStackedPR(cfg, githubmock, gitmock)
	output = &bytes.Buffer{}
	s.output = output
	input = &bytes.Buffer{}
	s.input = input
	return
}

// TODO(mattskl): revisit if I need merge queue.
func TestSPRBasicFlowFourCommitsQueue(t *testing.T) {
	fmt.Println("TestSPRBasicFlowFourCommitsQueue -- skipped")
	return

	s, gitmock, githubmock, _, output := makeTestObjects(t, "rebase")
	assert := require.New(t)
	ctx := context.Background()

	c1 := git.Commit{
		CommitID:   "00000001",
		CommitHash: "c100000000000000000000000000000000000000",
		Subject:    "test commit 1",
	}
	c2 := git.Commit{
		CommitID:   "00000002",
		CommitHash: "c200000000000000000000000000000000000000",
		Subject:    "test commit 2",
	}
	c3 := git.Commit{
		CommitID:   "00000003",
		CommitHash: "c300000000000000000000000000000000000000",
		Subject:    "test commit 3",
	}
	c4 := git.Commit{
		CommitID:   "00000004",
		CommitHash: "c400000000000000000000000000000000000000",
		Subject:    "test commit 4",
	}

	// 'git spr status' :: StatusPullRequest
	githubmock.ExpectGetInfo()
	s.StatusPullRequests(ctx)
	assert.Equal("pull request stack is empty\n", output.String())
	output.Reset()

	// 'git spr update' :: UpdatePullRequest :: commits=[c1]
	githubmock.ExpectGetInfo()
	gitmock.ExpectFetch()
	gitmock.ExpectLogAndRespond([]*git.Commit{&c1})
	gitmock.ExpectPushCommits([]*git.Commit{&c1})
	githubmock.ExpectCreatePullRequest(c1, nil)
	githubmock.ExpectGetAssignableUsers()
	githubmock.ExpectAddReviewers([]string{mockclient.NobodyUserID})
	githubmock.ExpectUpdatePullRequest(c1, nil)
	githubmock.ExpectGetInfo()
	s.UpdatePullRequests(ctx, []string{mockclient.NobodyLogin}, nil)
	fmt.Printf("OUT: %s\n", output.String())
	assert.Equal("[vvvv]   1 : test commit 1\n", output.String())
	output.Reset()

	// 'git spr update' :: UpdatePullRequest :: commits=[c1, c2]
	githubmock.ExpectGetInfo()
	gitmock.ExpectFetch()
	gitmock.ExpectLogAndRespond([]*git.Commit{&c2, &c1})
	gitmock.ExpectPushCommits([]*git.Commit{&c2})
	githubmock.ExpectCreatePullRequest(c2, &c1)
	githubmock.ExpectGetAssignableUsers()
	githubmock.ExpectAddReviewers([]string{mockclient.NobodyUserID})
	githubmock.ExpectUpdatePullRequest(c1, nil)
	githubmock.ExpectUpdatePullRequest(c2, &c1)
	githubmock.ExpectGetInfo()
	s.UpdatePullRequests(ctx, []string{mockclient.NobodyLogin}, nil)
	lines := strings.Split(output.String(), "\n")
	fmt.Printf("OUT: %s\n", output.String())
	assert.Equal("warning: not updating reviewers for PR #1", lines[0])
	assert.Equal("[vvvv]   1 : test commit 2", lines[1])
	assert.Equal("[vvvv]   1 : test commit 1", lines[2])
	output.Reset()

	// 'git spr update' :: UpdatePullRequest :: commits=[c1, c2, c3, c4]
	githubmock.ExpectGetInfo()
	gitmock.ExpectFetch()
	gitmock.ExpectLogAndRespond([]*git.Commit{&c4, &c3, &c2, &c1})
	gitmock.ExpectPushCommits([]*git.Commit{&c3, &c4})

	// For the first "create" call we should call GetAssignableUsers
	githubmock.ExpectCreatePullRequest(c3, &c2)
	githubmock.ExpectGetAssignableUsers()
	githubmock.ExpectAddReviewers([]string{mockclient.NobodyUserID})

	// For the first "create" call we should *not* call GetAssignableUsers
	githubmock.ExpectCreatePullRequest(c4, &c3)
	githubmock.ExpectAddReviewers([]string{mockclient.NobodyUserID})

	githubmock.ExpectUpdatePullRequest(c1, nil)
	githubmock.ExpectUpdatePullRequest(c2, &c1)
	githubmock.ExpectUpdatePullRequest(c3, &c2)
	githubmock.ExpectUpdatePullRequest(c4, &c3)
	githubmock.ExpectGetInfo()
	s.UpdatePullRequests(ctx, []string{mockclient.NobodyLogin}, nil)
	lines = strings.Split(output.String(), "\n")
	fmt.Printf("OUT: %s\n", output.String())
	assert.Equal([]string{
		"warning: not updating reviewers for PR #1",
		"warning: not updating reviewers for PR #1",
		"[vvvv]   1 : test commit 4",
		"[vvvv]   1 : test commit 3",
		"[vvvv]   1 : test commit 2",
		"[vvvv]   1 : test commit 1",
	}, lines[:6])
	output.Reset()

	// 'git spr merge --count 2' :: MergePullRequest :: commits=[a1, a2]
	githubmock.ExpectGetInfo()
	githubmock.ExpectUpdatePullRequest(c2, nil)
	githubmock.ExpectMergePullRequest(c2, genclient.PullRequestMergeMethod_REBASE)
	githubmock.ExpectCommentPullRequest(c1)
	githubmock.ExpectClosePullRequest(c1)
	githubmock.ExpectCommentPullRequest(c2)
	githubmock.ExpectClosePullRequest(c2)
	count := uint(2)
	s.MergePullRequests(ctx, &count)
	lines = strings.Split(output.String(), "\n")
	assert.Equal("MERGED   1 : test commit 1", lines[0])
	assert.Equal("MERGED   1 : test commit 2", lines[1])
	fmt.Printf("OUT: %s\n", output.String())
	output.Reset()

	githubmock.Info.PullRequests = githubmock.Info.PullRequests[1:]
	githubmock.Info.PullRequests[0].Merged = false
	githubmock.Info.PullRequests[0].Commits = append(githubmock.Info.PullRequests[0].Commits, c1, c2)

	gitmock.ExpectFetch()
	gitmock.ExpectLogAndRespond([]*git.Commit{&c4, &c3, &c2, &c1})
	gitmock.ExpectPushCommits([]*git.Commit{&c2, &c3, &c4})

	s.UpdatePullRequests(ctx, []string{mockclient.NobodyLogin}, nil)
	lines = strings.Split(output.String(), "\n")
	fmt.Printf("OUT: %s\n", output.String())
	assert.Equal([]string{
		"warning: not updating reviewers for PR #1",
		"warning: not updating reviewers for PR #1",
		"warning: not updating reviewers for PR #1",
		"[vvvv]   1 : test commit 4",
		"[vvvv]   1 : test commit 3",
		"[vvvv] !   1 : test commit 2",
	}, lines[:6])
	output.Reset()

	// 'git spr merge' :: MergePullRequest :: commits=[a2, a3, a4]
	gitmock.ExpectLocalBranch("master")
	githubmock.ExpectGetInfo()
	gitmock.ExpectLocalBranch("master")
	githubmock.ExpectUpdatePullRequest(c4, nil)
	githubmock.ExpectMergePullRequest(c4, genclient.PullRequestMergeMethod_REBASE)

	githubmock.ExpectCommentPullRequest(c2)
	githubmock.ExpectClosePullRequest(c2)
	githubmock.ExpectCommentPullRequest(c3)
	githubmock.ExpectClosePullRequest(c3)

	githubmock.Info.PullRequests[0].InQueue = true

	s.MergePullRequests(ctx, nil)
	lines = strings.Split(output.String(), "\n")
	assert.Equal("MERGED .   1 : test commit 2", lines[0])
	assert.Equal("MERGED   1 : test commit 3", lines[1])
	assert.Equal("MERGED   1 : test commit 4", lines[2])
	fmt.Printf("OUT: %s\n", output.String())
	output.Reset()
}

func TestSPRBasicFlowFourCommits(t *testing.T) {
	fmt.Println("TestSPRBasicFlowFourCommits")
	s, gitmock, githubmock, _, output := makeTestObjects(t, "squash")
	assert := require.New(t)
	ctx := context.Background()

	c1 := git.Commit{
		CommitID:   "00000001",
		CommitHash: "c100000000000000000000000000000000000000",
		Subject:    "test commit 1",
	}
	c2 := git.Commit{
		CommitID:   "00000002",
		CommitHash: "c200000000000000000000000000000000000000",
		Subject:    "test commit 2",
	}
	c3 := git.Commit{
		CommitID:   "00000003",
		CommitHash: "c300000000000000000000000000000000000000",
		Subject:    "test commit 3",
	}
	c4 := git.Commit{
		CommitID:   "00000004",
		CommitHash: "c400000000000000000000000000000000000000",
		Subject:    "test commit 4",
	}

	// 'git spr status' :: StatusPullRequest
	githubmock.ExpectGetInfo()
	s.StatusPullRequests(ctx)
	assert.Equal("pull request stack is empty\n", output.String())
	output.Reset()

	// 'git spr update' :: UpdatePullRequest :: commits=[c1]
	githubmock.ExpectGetInfo()
	gitmock.ExpectFetch()
	gitmock.ExpectLogAndRespond([]*git.Commit{&c1})
	gitmock.ExpectPushCommits([]*git.Commit{&c1})
	githubmock.ExpectCreatePullRequest(c1, nil)
	githubmock.ExpectGetAssignableUsers()
	githubmock.ExpectAddReviewers([]string{mockclient.NobodyUserID})
	githubmock.ExpectUpdatePullRequest(c1, nil)
	githubmock.ExpectGetInfo()
	s.UpdatePullRequests(ctx, []string{mockclient.NobodyLogin}, nil)
	fmt.Printf("OUT: %s\n", output.String())
	assert.Equal("[vvvv]   1 : test commit 1\n", output.String())
	output.Reset()

	// 'git spr update' :: UpdatePullRequest :: commits=[c1, c2]
	githubmock.ExpectGetInfo()
	gitmock.ExpectFetch()
	gitmock.ExpectLogAndRespond([]*git.Commit{&c2, &c1})
	gitmock.ExpectPushCommits([]*git.Commit{&c2})
	githubmock.ExpectCreatePullRequest(c2, &c1)
	githubmock.ExpectGetAssignableUsers()
	githubmock.ExpectAddReviewers([]string{mockclient.NobodyUserID})
	githubmock.ExpectUpdatePullRequest(c1, nil)
	githubmock.ExpectUpdatePullRequest(c2, &c1)
	githubmock.ExpectGetInfo()
	s.UpdatePullRequests(ctx, []string{mockclient.NobodyLogin}, nil)
	lines := strings.Split(output.String(), "\n")
	fmt.Printf("OUT: %s\n", output.String())
	assert.Equal("warning: not updating reviewers for PR #1", lines[0])
	assert.Equal("[vvvv]   1 : test commit 2", lines[1])
	assert.Equal("[vvvv]   1 : test commit 1", lines[2])
	output.Reset()

	// 'git spr update' :: UpdatePullRequest :: commits=[c1, c2, c3, c4]
	githubmock.ExpectGetInfo()
	gitmock.ExpectFetch()
	gitmock.ExpectLogAndRespond([]*git.Commit{&c4, &c3, &c2, &c1})
	gitmock.ExpectPushCommits([]*git.Commit{&c3, &c4})

	// For the first "create" call we should call GetAssignableUsers
	githubmock.ExpectCreatePullRequest(c3, &c2)
	githubmock.ExpectGetAssignableUsers()
	githubmock.ExpectAddReviewers([]string{mockclient.NobodyUserID})

	// For the first "create" call we should *not* call GetAssignableUsers
	githubmock.ExpectCreatePullRequest(c4, &c3)
	githubmock.ExpectAddReviewers([]string{mockclient.NobodyUserID})

	githubmock.ExpectUpdatePullRequest(c1, nil)
	githubmock.ExpectUpdatePullRequest(c2, &c1)
	githubmock.ExpectUpdatePullRequest(c3, &c2)
	githubmock.ExpectUpdatePullRequest(c4, &c3)
	githubmock.ExpectGetInfo()
	s.UpdatePullRequests(ctx, []string{mockclient.NobodyLogin}, nil)
	lines = strings.Split(output.String(), "\n")
	fmt.Printf("OUT: %s\n", output.String())
	assert.Equal([]string{
		"warning: not updating reviewers for PR #1",
		"warning: not updating reviewers for PR #1",
		"[vvvv]   1 : test commit 4",
		"[vvvv]   1 : test commit 3",
		"[vvvv]   1 : test commit 2",
		"[vvvv]   1 : test commit 1",
	}, lines[:6])
	output.Reset()

	// 'git spr merge --count 1' :: MergePullRequest :: commits=[a1, a2, a3, a4]
	githubmock.ExpectGetInfo()
	githubmock.ExpectUpdatePullRequest(c1, nil)
	githubmock.ExpectMergePullRequest(c1, genclient.PullRequestMergeMethod_SQUASH)
	s.MergePullRequests(ctx, uintptr(1))
	lines = strings.Split(output.String(), "\n")
	assert.Equal("MERGED   1 : test commit 1", lines[0])
	fmt.Printf("OUT: %s\n", output.String())
	output.Reset()

	// Drop the first PR since it's merged, and update the second PR as having
	// 2 commits (c2 and c1).
	// Also update all the commit hashes
	githubmock.Info.PullRequests = githubmock.Info.PullRequests[1:]
	githubmock.Info.PullRequests[0].Commits = append(githubmock.Info.PullRequests[0].Commits, c1)
	c2.CommitHash = "c20000000000000000000000000000000000000a"
	c3.CommitHash = "c30000000000000000000000000000000000000a"
	c4.CommitHash = "c40000000000000000000000000000000000000a"

	// `git spr update --count 1`
	githubmock.ExpectGetInfo()
	gitmock.ExpectFetch()
	// Git fetch+rebase should drop c1 locally, since it'll be detected as merged.
	gitmock.ExpectLogAndRespond([]*git.Commit{&c4, &c3, &c2})
	// Push to update to synced commit, just c2 since --count 1
	gitmock.ExpectPushCommits([]*git.Commit{&c2})
	githubmock.ExpectUpdatePullRequest(c2, nil)
	githubmock.ExpectGetInfo()

	s.UpdatePullRequests(ctx, []string{mockclient.NobodyLogin}, uintptr(1))
	lines = strings.Split(output.String(), "\n")
	fmt.Printf("OUT: %s\n", output.String())
	assert.Equal([]string{
		"warning: not updating reviewers for PR #1",
		"[vvvv]   1 : test commit 4",
		"[vvvv]   1 : test commit 3",
		"[vvvv]   1 : test commit 2",
	}, lines[:4])
	output.Reset()

	// `git spr update`
	githubmock.ExpectGetInfo()
	gitmock.ExpectFetch()
	gitmock.ExpectLogAndRespond([]*git.Commit{&c4, &c3, &c2})
	gitmock.ExpectPushCommits([]*git.Commit{&c3, &c4})
	githubmock.ExpectUpdatePullRequest(c3, &c2)
	githubmock.ExpectUpdatePullRequest(c4, &c3)
	githubmock.ExpectGetInfo()

	s.UpdatePullRequests(ctx, []string{mockclient.NobodyLogin}, nil)
	lines = strings.Split(output.String(), "\n")
	fmt.Printf("OUT: %s\n", output.String())
	assert.Equal([]string{
		"warning: not updating reviewers for PR #1",
		"warning: not updating reviewers for PR #1",
		"warning: not updating reviewers for PR #1",
		"[vvvv]   1 : test commit 4",
		"[vvvv]   1 : test commit 3",
		"[vvvv]   1 : test commit 2",
	}, lines[:6])
	output.Reset()
}

func TestSPRMergeCount(t *testing.T) {
	fmt.Println("TestSPRMergeCount")
	s, gitmock, githubmock, _, output := makeTestObjects(t, "rebase")
	assert := require.New(t)
	ctx := context.Background()

	c1 := git.Commit{
		CommitID:   "00000001",
		CommitHash: "c100000000000000000000000000000000000000",
		Subject:    "test commit 1",
	}
	c2 := git.Commit{
		CommitID:   "00000002",
		CommitHash: "c200000000000000000000000000000000000000",
		Subject:    "test commit 2",
	}
	c3 := git.Commit{
		CommitID:   "00000003",
		CommitHash: "c300000000000000000000000000000000000000",
		Subject:    "test commit 3",
	}
	c4 := git.Commit{
		CommitID:   "00000004",
		CommitHash: "c400000000000000000000000000000000000000",
		Subject:    "test commit 4",
	}

	// 'git spr update' :: UpdatePullRequest :: commits=[c1, c2, c3, c4]
	githubmock.ExpectGetInfo()
	gitmock.ExpectFetch()
	gitmock.ExpectLogAndRespond([]*git.Commit{&c4, &c3, &c2, &c1})
	gitmock.ExpectPushCommits([]*git.Commit{&c1, &c2, &c3, &c4})
	// For the first "create" call we should call GetAssignableUsers
	githubmock.ExpectCreatePullRequest(c1, nil)
	githubmock.ExpectGetAssignableUsers()
	githubmock.ExpectAddReviewers([]string{mockclient.NobodyUserID})
	githubmock.ExpectCreatePullRequest(c2, &c1)
	githubmock.ExpectAddReviewers([]string{mockclient.NobodyUserID})
	githubmock.ExpectCreatePullRequest(c3, &c2)
	githubmock.ExpectAddReviewers([]string{mockclient.NobodyUserID})
	githubmock.ExpectCreatePullRequest(c4, &c3)
	githubmock.ExpectAddReviewers([]string{mockclient.NobodyUserID})
	githubmock.ExpectUpdatePullRequest(c1, nil)
	githubmock.ExpectUpdatePullRequest(c2, &c1)
	githubmock.ExpectUpdatePullRequest(c3, &c2)
	githubmock.ExpectUpdatePullRequest(c4, &c3)
	githubmock.ExpectGetInfo()
	s.UpdatePullRequests(ctx, []string{mockclient.NobodyLogin}, nil)
	lines := strings.Split(output.String(), "\n")
	fmt.Printf("OUT: %s\n", output.String())
	assert.Equal([]string{
		"[vvvv]   1 : test commit 4",
		"[vvvv]   1 : test commit 3",
		"[vvvv]   1 : test commit 2",
		"[vvvv]   1 : test commit 1",
	}, lines[:4])
	output.Reset()

	// 'git spr merge --count 2' :: MergePullRequest :: commits=[a1, a2, a3, a4]
	githubmock.ExpectGetInfo()
	githubmock.ExpectUpdatePullRequest(c2, nil)
	githubmock.ExpectMergePullRequest(c2, genclient.PullRequestMergeMethod_REBASE)
	githubmock.ExpectCommentPullRequest(c1)
	githubmock.ExpectClosePullRequest(c1)
	s.MergePullRequests(ctx, uintptr(2))
	lines = strings.Split(output.String(), "\n")
	assert.Equal("MERGED   1 : test commit 1", lines[0])
	assert.Equal("MERGED   1 : test commit 2", lines[1])
	fmt.Printf("OUT: %s\n", output.String())
	output.Reset()
}

func TestSPRAmendCommit(t *testing.T) {
	fmt.Println("TestSPRAmendCommit")
	s, gitmock, githubmock, _, output := makeTestObjects(t, "rebase")
	assert := require.New(t)
	ctx := context.Background()

	c1 := git.Commit{
		CommitID:   "00000001",
		CommitHash: "c100000000000000000000000000000000000000",
		Subject:    "test commit 1",
	}
	c2 := git.Commit{
		CommitID:   "00000002",
		CommitHash: "c200000000000000000000000000000000000000",
		Subject:    "test commit 2",
	}

	// 'git spr state' :: StatusPullRequest
	githubmock.ExpectGetInfo()
	s.StatusPullRequests(ctx)
	assert.Equal("pull request stack is empty\n", output.String())
	output.Reset()

	// 'git spr update' :: UpdatePullRequest :: commits=[c1, c2]
	githubmock.ExpectGetInfo()
	gitmock.ExpectFetch()
	gitmock.ExpectLogAndRespond([]*git.Commit{&c2, &c1})
	gitmock.ExpectPushCommits([]*git.Commit{&c1, &c2})
	githubmock.ExpectCreatePullRequest(c1, nil)
	githubmock.ExpectCreatePullRequest(c2, &c1)
	githubmock.ExpectUpdatePullRequest(c1, nil)
	githubmock.ExpectUpdatePullRequest(c2, &c1)
	githubmock.ExpectGetInfo()
	s.UpdatePullRequests(ctx, nil, nil)
	fmt.Printf("OUT: %s\n", output.String())
	lines := strings.Split(output.String(), "\n")
	assert.Equal("[vvvv]   1 : test commit 2", lines[0])
	assert.Equal("[vvvv]   1 : test commit 1", lines[1])
	output.Reset()

	// amend commit c2
	c2.CommitHash = "c201000000000000000000000000000000000000"
	// 'git spr update' :: UpdatePullRequest :: commits=[c1, c2]
	githubmock.ExpectGetInfo()
	gitmock.ExpectFetch()
	gitmock.ExpectLogAndRespond([]*git.Commit{&c2, &c1})
	gitmock.ExpectPushCommits([]*git.Commit{&c2})
	githubmock.ExpectUpdatePullRequest(c1, nil)
	githubmock.ExpectUpdatePullRequest(c2, &c1)
	githubmock.ExpectGetInfo()
	s.UpdatePullRequests(ctx, nil, nil)
	lines = strings.Split(output.String(), "\n")
	fmt.Printf("OUT: %s\n", output.String())
	assert.Equal("[vvvv]   1 : test commit 2", lines[0])
	assert.Equal("[vvvv]   1 : test commit 1", lines[1])
	output.Reset()

	// amend commit c1
	c1.CommitHash = "c101000000000000000000000000000000000000"
	c2.CommitHash = "c202000000000000000000000000000000000000"
	// 'git spr update' :: UpdatePullRequest :: commits=[c1, c2]
	githubmock.ExpectGetInfo()
	gitmock.ExpectFetch()
	gitmock.ExpectLogAndRespond([]*git.Commit{&c2, &c1})
	gitmock.ExpectPushCommits([]*git.Commit{&c1, &c2})
	githubmock.ExpectUpdatePullRequest(c1, nil)
	githubmock.ExpectUpdatePullRequest(c2, &c1)
	githubmock.ExpectGetInfo()
	s.UpdatePullRequests(ctx, nil, nil)
	lines = strings.Split(output.String(), "\n")
	fmt.Printf("OUT: %s\n", output.String())
	assert.Equal("[vvvv]   1 : test commit 2", lines[0])
	assert.Equal("[vvvv]   1 : test commit 1", lines[1])
	output.Reset()

	// 'git spr merge' :: MergePullRequest :: commits=[a1, a2]
	githubmock.ExpectGetInfo()
	githubmock.ExpectUpdatePullRequest(c2, nil)
	githubmock.ExpectMergePullRequest(c2, genclient.PullRequestMergeMethod_REBASE)
	githubmock.ExpectCommentPullRequest(c1)
	githubmock.ExpectClosePullRequest(c1)
	githubmock.ExpectCommentPullRequest(c2)
	githubmock.ExpectClosePullRequest(c2)
	s.MergePullRequests(ctx, nil)
	lines = strings.Split(output.String(), "\n")
	assert.Equal("MERGED   1 : test commit 1", lines[0])
	assert.Equal("MERGED   1 : test commit 2", lines[1])
	fmt.Printf("OUT: %s\n", output.String())
	output.Reset()
}

func TestSPRReorderCommit(t *testing.T) {
	fmt.Println("TestSPRReorderCommit")
	s, gitmock, githubmock, _, output := makeTestObjects(t, "rebase")
	assert := require.New(t)
	ctx := context.Background()

	c1 := git.Commit{
		CommitID:   "00000001",
		CommitHash: "c100000000000000000000000000000000000000",
		Subject:    "test commit 1",
	}
	c2 := git.Commit{
		CommitID:   "00000002",
		CommitHash: "c200000000000000000000000000000000000000",
		Subject:    "test commit 2",
	}
	c3 := git.Commit{
		CommitID:   "00000003",
		CommitHash: "c300000000000000000000000000000000000000",
		Subject:    "test commit 3",
	}
	c4 := git.Commit{
		CommitID:   "00000004",
		CommitHash: "c400000000000000000000000000000000000000",
		Subject:    "test commit 4",
	}
	c5 := git.Commit{
		CommitID:   "00000005",
		CommitHash: "c500000000000000000000000000000000000000",
		Subject:    "test commit 5",
	}

	// 'git spr status' :: StatusPullRequest
	githubmock.ExpectGetInfo()
	s.StatusPullRequests(ctx)
	assert.Equal("pull request stack is empty\n", output.String())
	output.Reset()

	// 'git spr update' :: UpdatePullRequest :: commits=[c1, c2, c3, c4]
	githubmock.ExpectGetInfo()
	gitmock.ExpectFetch()
	gitmock.ExpectLogAndRespond([]*git.Commit{&c4, &c3, &c2, &c1})
	gitmock.ExpectPushCommits([]*git.Commit{&c1, &c2, &c3, &c4})
	githubmock.ExpectCreatePullRequest(c1, nil)
	githubmock.ExpectCreatePullRequest(c2, &c1)
	githubmock.ExpectCreatePullRequest(c3, &c2)
	githubmock.ExpectCreatePullRequest(c4, &c3)
	githubmock.ExpectUpdatePullRequest(c1, nil)
	githubmock.ExpectUpdatePullRequest(c2, &c1)
	githubmock.ExpectUpdatePullRequest(c3, &c2)
	githubmock.ExpectUpdatePullRequest(c4, &c3)
	githubmock.ExpectGetInfo()
	s.UpdatePullRequests(ctx, nil, nil)
	fmt.Printf("OUT: %s\n", output.String())
	lines := strings.Split(output.String(), "\n")
	assert.Equal("[vvvv]   1 : test commit 4", lines[0])
	assert.Equal("[vvvv]   1 : test commit 3", lines[1])
	assert.Equal("[vvvv]   1 : test commit 2", lines[2])
	assert.Equal("[vvvv]   1 : test commit 1", lines[3])
	output.Reset()

	// 'git spr update' :: UpdatePullRequest :: commits=[c2, c4, c1, c3]
	githubmock.ExpectGetInfo()
	gitmock.ExpectFetch()
	gitmock.ExpectLogAndRespond([]*git.Commit{&c3, &c1, &c4, &c2})
	githubmock.ExpectUpdatePullRequest(c1, nil)
	githubmock.ExpectUpdatePullRequest(c2, nil)
	githubmock.ExpectUpdatePullRequest(c3, nil)
	githubmock.ExpectUpdatePullRequest(c4, nil)
	// reorder commits
	c1.CommitHash = "c101000000000000000000000000000000000000"
	c2.CommitHash = "c201000000000000000000000000000000000000"
	c3.CommitHash = "c301000000000000000000000000000000000000"
	c4.CommitHash = "c401000000000000000000000000000000000000"
	gitmock.ExpectPushCommits([]*git.Commit{&c2, &c4, &c1, &c3})
	githubmock.ExpectUpdatePullRequest(c2, nil)
	githubmock.ExpectUpdatePullRequest(c4, &c2)
	githubmock.ExpectUpdatePullRequest(c1, &c4)
	githubmock.ExpectUpdatePullRequest(c3, &c1)
	githubmock.ExpectGetInfo()
	s.UpdatePullRequests(ctx, nil, nil)
	fmt.Printf("OUT: %s\n", output.String())
	// TODO : Need to update pull requests in GetInfo expect to get this check to work
	// lines = strings.Split(output.String(), "\n")
	//assert.Equal("[vvvv]   1 : test commit 3", lines[0])
	//assert.Equal("[vvvv]   1 : test commit 1", lines[1])
	//assert.Equal("[vvvv]   1 : test commit 4", lines[2])
	//assert.Equal("[vvvv]   1 : test commit 2", lines[3])
	output.Reset()

	// 'git spr update' :: UpdatePullRequest :: commits=[c5, c1, c2, c3, c4]
	githubmock.ExpectGetInfo()
	gitmock.ExpectFetch()
	gitmock.ExpectLogAndRespond([]*git.Commit{&c1, &c2, &c3, &c4, &c5})
	githubmock.ExpectUpdatePullRequest(c1, nil)
	githubmock.ExpectUpdatePullRequest(c2, nil)
	githubmock.ExpectUpdatePullRequest(c3, nil)
	githubmock.ExpectUpdatePullRequest(c4, nil)
	// reorder commits
	c1.CommitHash = "c102000000000000000000000000000000000000"
	c2.CommitHash = "c202000000000000000000000000000000000000"
	c3.CommitHash = "c302000000000000000000000000000000000000"
	c4.CommitHash = "c402000000000000000000000000000000000000"
	gitmock.ExpectPushCommits([]*git.Commit{&c5, &c4, &c3, &c2, &c1})
	githubmock.ExpectCreatePullRequest(c5, nil)
	githubmock.ExpectUpdatePullRequest(c5, nil)
	githubmock.ExpectUpdatePullRequest(c4, &c5)
	githubmock.ExpectUpdatePullRequest(c3, &c4)
	githubmock.ExpectUpdatePullRequest(c2, &c3)
	githubmock.ExpectUpdatePullRequest(c1, &c2)
	githubmock.ExpectGetInfo()
	s.UpdatePullRequests(ctx, nil, nil)
	fmt.Printf("OUT: %s\n", output.String())
	// TODO : Need to update pull requests in GetInfo expect to get this check to work
	// lines = strings.Split(output.String(), "\n")
	//assert.Equal("[vvvv]   1 : test commit 5", lines[0])
	//assert.Equal("[vvvv]   1 : test commit 4", lines[1])
	//assert.Equal("[vvvv]   1 : test commit 3", lines[2])
	//assert.Equal("[vvvv]   1 : test commit 2", lines[3])
	//assert.Equal("[vvvv]   1 : test commit 1", lines[4])
	output.Reset()

	// TODO : add a call to merge and check merge order
}

// TODO(mattskl): consider usefulness of SPR deleting commits for me.
// So far, it's only been unwanted and unintuitive.
func TestSPRDeleteCommit(t *testing.T) {
	fmt.Println("TestSPRDeleteCommit -- skipped")
	return

	s, gitmock, githubmock, _, output := makeTestObjects(t, "rebase")
	assert := require.New(t)
	ctx := context.Background()

	c1 := git.Commit{
		CommitID:   "00000001",
		CommitHash: "c100000000000000000000000000000000000000",
		Subject:    "test commit 1",
	}
	c2 := git.Commit{
		CommitID:   "00000002",
		CommitHash: "c200000000000000000000000000000000000000",
		Subject:    "test commit 2",
	}
	c3 := git.Commit{
		CommitID:   "00000003",
		CommitHash: "c300000000000000000000000000000000000000",
		Subject:    "test commit 3",
	}
	c4 := git.Commit{
		CommitID:   "00000004",
		CommitHash: "c400000000000000000000000000000000000000",
		Subject:    "test commit 4",
	}

	// 'git spr status' :: StatusPullRequest
	githubmock.ExpectGetInfo()
	s.StatusPullRequests(ctx)
	assert.Equal("pull request stack is empty\n", output.String())
	output.Reset()

	// 'git spr update' :: UpdatePullRequest :: commits=[c1, c2, c3, c4]
	githubmock.ExpectGetInfo()
	gitmock.ExpectFetch()
	gitmock.ExpectLogAndRespond([]*git.Commit{&c4, &c3, &c2, &c1})
	gitmock.ExpectPushCommits([]*git.Commit{&c1, &c2, &c3, &c4})
	githubmock.ExpectCreatePullRequest(c1, nil)
	githubmock.ExpectCreatePullRequest(c2, &c1)
	githubmock.ExpectCreatePullRequest(c3, &c2)
	githubmock.ExpectCreatePullRequest(c4, &c3)
	githubmock.ExpectUpdatePullRequest(c1, nil)
	githubmock.ExpectUpdatePullRequest(c2, &c1)
	githubmock.ExpectUpdatePullRequest(c3, &c2)
	githubmock.ExpectUpdatePullRequest(c4, &c3)
	githubmock.ExpectGetInfo()

	s.UpdatePullRequests(ctx, nil, nil)
	fmt.Printf("OUT: %s\n", output.String())
	lines := strings.Split(output.String(), "\n")
	assert.Equal("[vvvv]   1 : test commit 4", lines[0])
	assert.Equal("[vvvv]   1 : test commit 3", lines[1])
	assert.Equal("[vvvv]   1 : test commit 2", lines[2])
	assert.Equal("[vvvv]   1 : test commit 1", lines[3])
	output.Reset()

	// 'git spr update' :: UpdatePullRequest :: commits=[c2, c4, c1, c3]
	githubmock.ExpectGetInfo()
	gitmock.ExpectFetch()
	gitmock.ExpectLogAndRespond([]*git.Commit{&c4, &c1})
	githubmock.ExpectCommentPullRequest(c2)
	githubmock.ExpectClosePullRequest(c2)
	githubmock.ExpectCommentPullRequest(c3)
	githubmock.ExpectClosePullRequest(c3)
	// update commits
	c1.CommitHash = "c101000000000000000000000000000000000000"
	c4.CommitHash = "c401000000000000000000000000000000000000"
	githubmock.ExpectUpdatePullRequest(c1, nil)
	githubmock.ExpectUpdatePullRequest(c4, &c1)
	gitmock.ExpectPushCommits([]*git.Commit{&c1, &c4})
	githubmock.ExpectGetInfo()
	s.UpdatePullRequests(ctx, nil, nil)
	fmt.Printf("OUT: %s\n", output.String())
	// TODO : Need to update pull requests in GetInfo expect to get this check to work
	// lines = strings.Split(output.String(), "\n")
	//assert.Equal("[vvvv]   1 : test commit 3", lines[0])
	//assert.Equal("[vvvv]   1 : test commit 1", lines[1])
	//assert.Equal("[vvvv]   1 : test commit 4", lines[2])
	//assert.Equal("[vvvv]   1 : test commit 2", lines[3])
	output.Reset()

	// TODO : add a call to merge and check merge order
}

func TestAmendNoCommits(t *testing.T) {
	fmt.Println("TestAmendNoCommits")
	s, gitmock, _, _, output := makeTestObjects(t, "rebase")
	assert := require.New(t)
	ctx := context.Background()

	gitmock.ExpectLogAndRespond([]*git.Commit{})
	s.AmendCommit(ctx)
	assert.Equal("No commits to amend\n", output.String())
}

func TestAmendOneCommit(t *testing.T) {
	fmt.Println("TestAmendOneCommit")
	s, gitmock, _, input, output := makeTestObjects(t, "rebase")
	assert := require.New(t)
	ctx := context.Background()

	c1 := git.Commit{
		CommitID:   "00000001",
		CommitHash: "c100000000000000000000000000000000000000",
		Subject:    "test commit 1",
	}
	gitmock.ExpectLogAndRespond([]*git.Commit{&c1})
	gitmock.ExpectFixup(c1.CommitHash)
	input.WriteString("1")
	s.AmendCommit(ctx)
	assert.Equal(" 1 : 00000001 : test commit 1\nCommit to amend (1): ", output.String())
}

func TestAmendTwoCommits(t *testing.T) {
	fmt.Println("TestAmendTwoCommits")
	s, gitmock, _, input, output := makeTestObjects(t, "rebase")
	assert := require.New(t)
	ctx := context.Background()

	c1 := git.Commit{
		CommitID:   "00000001",
		CommitHash: "c100000000000000000000000000000000000000",
		Subject:    "test commit 1",
	}
	c2 := git.Commit{
		CommitID:   "00000002",
		CommitHash: "c200000000000000000000000000000000000000",
		Subject:    "test commit 2",
	}
	gitmock.ExpectLogAndRespond([]*git.Commit{&c1, &c2})
	gitmock.ExpectFixup(c2.CommitHash)
	input.WriteString("1")
	s.AmendCommit(ctx)
	assert.Equal(" 2 : 00000001 : test commit 1\n 1 : 00000002 : test commit 2\nCommit to amend (1-2): ", output.String())
}

func TestAmendInvalidInput(t *testing.T) {
	fmt.Println("TestAmendInvalidInput")
	s, gitmock, _, input, output := makeTestObjects(t, "rebase")
	assert := require.New(t)
	ctx := context.Background()

	c1 := git.Commit{
		CommitID:   "00000001",
		CommitHash: "c100000000000000000000000000000000000000",
		Subject:    "test commit 1",
	}

	gitmock.ExpectLogAndRespond([]*git.Commit{&c1})
	input.WriteString("a")
	s.AmendCommit(ctx)
	assert.Equal(" 1 : 00000001 : test commit 1\nCommit to amend (1): Invalid input\n", output.String())
	output.Reset()

	gitmock.ExpectLogAndRespond([]*git.Commit{&c1})
	input.WriteString("0")
	s.AmendCommit(ctx)
	assert.Equal(" 1 : 00000001 : test commit 1\nCommit to amend (1): Invalid input\n", output.String())
	output.Reset()

	gitmock.ExpectLogAndRespond([]*git.Commit{&c1})
	input.WriteString("2")
	s.AmendCommit(ctx)
	assert.Equal(" 1 : 00000001 : test commit 1\nCommit to amend (1): Invalid input\n", output.String())
	output.Reset()
}

func uintptr(a uint) *uint {
	return &a
}
