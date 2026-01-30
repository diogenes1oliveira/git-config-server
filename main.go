package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/jessevdk/go-flags"
	"github.com/joho/godotenv"
	shellquote "github.com/kballard/go-shellquote"
)

var Options struct {
	RepoUrl            string `short:"u" long:"url" description:"Git URL" env:"GIT_URL"`
	RepoFolder         string `short:"r" long:"repo-folder" required:"false" default:"." description:"Git repo folder" env:"GIT_REPO_FOLDER"`
	LocalFolder        string `short:"l" long:"local-folder" required:"false" default:"." description:"Git local folder" env:"GIT_LOCAL_FOLDER"`
	RepoBranch         string `short:"b" long:"branch" default:"master" description:"Git branch" env:"GIT_BRANCH"`
	Username           string `long:"username" description:"Git username" env:"GIT_USERNAME"`
	Password           string `long:"password" description:"Git password" env:"GIT_PASSWORD"`
	UpdatePeriod       int    `long:"update-period" default:"60" description:"Update period in seconds" env:"GIT_UPDATE_PERIOD"`
	PreUpdateCommand   string `long:"pre-update-command" default:"true" description:"Shell command to run before restarting the application after an update. The working directory will be set to the local repo folder" env:"PRE_UPDATE_COMMAND"`
	RestartCommand     string `long:"restart-command" default:"true" description:"Shell command to run before restarting the application after an update. The working directory will be set to the local repo folder" env:"RESTART_COMMAND"`
	PreUpdateRunner    string `long:"pre-update-runner" default:"bash" description:"Shell to run the pre-update command" env:"PRE_UPDATE_RUNNER"`
	WebhookPort        int    `long:"webhook-port" default:"0" description:"Port to bind the webhook server to" env:"WEBHOOK_PORT"`
	WebhookTokenValue  string `long:"webhook-token-value" default:"" description:"Token value to authenticate requests" env:"WEBHOOK_TOKEN_VALUE"`
	WebhookTokenHeader string `long:"webhook-token-header" default:"" description:"Header with the token value" env:"WEBHOOK_TOKEN_HEADER"`

	Cmd []string `no-flag:"yes"`
}

func main() {
	envFile := os.Getenv("ENV_FILE")
	if envFile == "" {
		envFile = ".env"
	}
	err := godotenv.Load(envFile)
	if err != nil {
		log.Println("failed to load .env")
	}

	parser := flags.NewParser(&Options, flags.Default)
	args, err := parser.Parse()
	if err != nil {
		panic(err)
	}
	if len(args) == 0 {
		log.Fatalf("No command specified")
	}

	if Options.RepoUrl == "" {
		doExec(args...)
	}

	var beforeUpdate func() error

	if Options.PreUpdateCommand != "" {
		beforeUpdate = func() error {
			return runShellCommand(Options.PreUpdateCommand, Options.PreUpdateRunner, Options.LocalFolder)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var restartArgs []string
	if len(Options.RestartCommand) > 0 {
		restartArgs, err = shellquote.Split(Options.RestartCommand)
		if err != nil {
			log.Fatalf("failed to parse restart command: %v\n", err)
		}
	}
	command := NewCommand(ctx, args, restartArgs)
	gitRepo := NewGitRepo(Options.RepoUrl, Options.RepoBranch, Options.RepoFolder, Options.Username, Options.Password)

	updateCh := make(chan struct{}, 5)

	if Options.WebhookPort != 0 {
		err := StartWebhookServer(ctx, Options.WebhookPort, Options.WebhookTokenHeader, Options.WebhookTokenValue, func() error {
			updateCh <- struct{}{}
			return nil
		})
		if err != nil {
			log.Fatalf("failed to start webhook server: %v\n", err)
		}
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go (func() {
		for range c {
			log.Printf("interrupt received\n")
			cancel()
		}
	})()

	gitInitialized := false

	ok, err := InitializeGit(gitRepo, beforeUpdate)
	if err != nil {
		log.Fatalf("failed to initialize monitor: %v\n", err)
	}
	if ok {
		gitInitialized = true
	}

	err = command.Start()
	if err != nil {
		log.Fatalf("command failed to even start: %v\n", err)
	}

	done := false

	for !done {
		log.Printf("waiting %d seconds before checking again\n", Options.UpdatePeriod)
		select {
		case <-ctx.Done():
			log.Printf("interrupted, skipping update")
			done = true
			continue
		case <-updateCh:
		case <-time.After(time.Duration(Options.UpdatePeriod) * time.Second):
			// pass
		}

		if !gitInitialized {
			log.Printf("trying to initialize monitor\n")
			ok, err := InitializeGit(gitRepo, beforeUpdate)
			if err != nil && ok {
				log.Printf("monitor initialized successfully\n")
				gitInitialized = true
			}
			continue
		} else {
			err := Check(gitRepo, command, beforeUpdate)
			if err != nil {
				log.Fatalf("failed to check: %v\n", err)
			}
		}
	}

	if err := command.Stop(); err != nil {
		log.Fatalf("stop command failed")
	}
}

func InitializeGit(gitRepo *GitRepo, beforeUpdate func() error) (bool, error) {
	err := os.MkdirAll(Options.LocalFolder, 0o775)
	if err != nil {
		return false, fmt.Errorf("failed to create local folder %s: %w", Options.LocalFolder, err)
	}

	ok := true
	_, err = gitRepo.Sync(Options.LocalFolder)
	if err != nil {
		log.Printf("failed to synchronize Git to %s: %v\n", Options.LocalFolder, err)
		ok = false
	}

	if beforeUpdate != nil {
		log.Println("running beforeUpdate func for the first time")
		if err := beforeUpdate(); err != nil {
			log.Printf("failed to run beforeUpdate func for the first time: %v\n", err)
			ok = false
		}
	}

	return ok, nil
}

func Check(gitRepo *GitRepo, command *Command, beforeUpdate func() error) error {
	changed, err := gitRepo.Sync(Options.LocalFolder)
	if err != nil {
		log.Printf("failed to check git repo to %s: %v\n", Options.LocalFolder, err)
		return nil
	}
	if changed {
		if beforeUpdate != nil {
			log.Println("running beforeUpdate func")
			err = beforeUpdate()
			if err != nil {
				log.Printf("failed to run beforeUpdate func: %v\n", err)
				return nil
			}
		}
		err := command.Restart()
		if err != nil {
			log.Printf("failed to restart command: %v\n", err)
			return nil
		}
	}
	return nil
}

func CheckErr(err error) {
	if err != nil {
		panic(err)
	}
}

func doExec(args ...string) {
	cmd := args[0]

	log.Printf("will now exec cmd=%s\n", cmd)
	path, err := exec.LookPath(cmd)
	if err != nil {
		log.Fatalf("Failed to find command: %v", err)
	}

	err = syscall.Exec(path, args, os.Environ())
	if err != nil {
		log.Fatalf("Failed to exec command: %v", err)
	}
}
