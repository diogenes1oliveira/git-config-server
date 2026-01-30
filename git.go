package main

import (
	"fmt"
	"log"
	"os"
	"path"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"
)

type GitRepo struct {
	URL               string
	Branch            string
	RepoFolder        string
	username          string
	password          string
	lastFetchedCommit string
}

func NewGitRepo(url, branch, repoFolder, username, password string) *GitRepo {
	return &GitRepo{
		URL:        url,
		Branch:     branch,
		RepoFolder: strings.TrimLeft(repoFolder, "/"),
		username:   username,
		password:   password,
	}
}

// GitSync checks the remote repository for changes and synchronizes it
func (gitRepo *GitRepo) Sync(localFolder string) (bool, error) {
	lastCommit, err := gitRepo.GetLastCommit()
	if err != nil {
		log.Printf("failed to get last commit: %v\n", err)
		return false, err
	}

	if gitRepo.lastFetchedCommit == lastCommit {
		log.Printf("No changes in %s\n", gitRepo.URL)
		return false, nil
	}

	err = gitRepo.Fetch(lastCommit, localFolder)
	if err != nil {
		log.Printf("failed to fetch last commit: %v\n", err)
		return false, err
	}

	gitRepo.lastFetchedCommit = lastCommit
	return true, nil
}

// Fetch fetches the files from the remote repository into a local folder
func (gitRepo *GitRepo) Fetch(commit, localFolder string) error {
	tmpDir, err := os.MkdirTemp("", "git")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	log.Printf("Fetching commit %s of %s\n", gitRepo.URL, commit)

	repo, err := git.PlainClone(tmpDir, false, &git.CloneOptions{
		URL:           gitRepo.URL,
		Depth:         1,
		SingleBranch:  true,
		ReferenceName: plumbing.NewBranchReferenceName(gitRepo.Branch),
		Auth: &http.BasicAuth{
			Username: gitRepo.username,
			Password: gitRepo.password,
		},
	})
	if err != nil {
		return err
	}

	hash, err := repo.ResolveRevision(plumbing.Revision(commit))
	if err != nil {
		return err
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return err
	}

	err = worktree.Checkout(&git.CheckoutOptions{
		Hash: *hash,
	})
	if err != nil {
		return err
	}

	log.Printf("Copying repo folder /%s to local folder %s\n", gitRepo.RepoFolder, localFolder)

	repoSourceFolder := path.Join(tmpDir, gitRepo.RepoFolder)
	err = SyncDirs(repoSourceFolder, localFolder)
	if err != nil {
		log.Printf("failed to copy folders: %v\n", err)
		return err
	}

	return nil
}

// GitGetLastCommit fetches the last known commit hash in the branch
func (gitRepo *GitRepo) GetLastCommit() (string, error) {
	log.Printf("Fetching branch %s of %s\n", gitRepo.URL, gitRepo.Branch)

	repo, err := git.Clone(memory.NewStorage(), nil, &git.CloneOptions{
		URL:           gitRepo.URL,
		Depth:         1,
		SingleBranch:  true,
		NoCheckout:    true,
		ReferenceName: plumbing.NewBranchReferenceName(gitRepo.Branch),
		Auth: &http.BasicAuth{
			Username: gitRepo.username,
			Password: gitRepo.password,
		},
	})
	if err != nil {
		return "", err
	}
	ref, err := repo.Head()
	if err != nil {
		return "", err
	}
	commit := ref.Hash().String()
	if commit == "" {
		return "", fmt.Errorf("could not get commit hash")
	}

	log.Printf("last hash in branch %s: %v\n", gitRepo.Branch, commit)
	return commit, nil
}
