package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/arbadacarbaYK/gitnostr"
	"github.com/arbadacarbaYK/gitnostr/protocol"
)

func repoCreate(cfg Config, pool *nostr.RelayPool) {
	flags := flag.NewFlagSet("repo create", flag.ContinueOnError)

	flags.Parse(os.Args[3:])

	repoName := flags.Args()[0]

	log.Println("repo create ", repoName)

	// NIP-34: Use kind 30617 with tags, content MUST be empty per spec
	// NOTE: Privacy is NOT encoded in NIP-34 events (per spec)
	// Privacy is enforced via the "maintainers" tag (NIP-34 spec) and bridge access control
	var tags nostr.Tags
	// Required "d" tag for NIP-34 replaceable events
	tags = append(tags, nostr.Tag{"d", repoName})
	
	// Optional: Add clone tag if GitSshBase is configured
	if cfg.GitSshBase != "" {
		// Convert git@host:path format to https:// if needed, or use as-is
		cloneUrl := cfg.GitSshBase
		if strings.HasPrefix(cloneUrl, "git@") {
			// Keep SSH format for clone tag (clients can normalize)
			tags = append(tags, nostr.Tag{"clone", cloneUrl})
		} else {
			tags = append(tags, nostr.Tag{"clone", cloneUrl})
		}
	}

	_, statuses, err := pool.PublishEvent(&nostr.Event{
		CreatedAt: time.Now(),
		Kind:      protocol.KindRepositoryNIP34, // NIP-34: Use kind 30617
		Tags:      tags,
		Content:   "", // NIP-34: Content MUST be empty - all metadata in tags
	})
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	publishSuccess := false

	for {
		select {
		case <-ctx.Done():
			if !publishSuccess {
				fmt.Printf("repository was not published")
				os.Exit(1)
			}
			return
		case status := <-statuses:
			switch status.Status {
			case nostr.PublishStatusSent:
				publishSuccess = true
				fmt.Printf("published repository to '%s'.\n", status.Relay)
			case nostr.PublishStatusFailed:
				fmt.Printf("failed to publish repository to '%s'.\n", status.Relay)
			case nostr.PublishStatusSucceeded:
				publishSuccess = true
				fmt.Printf("published repository to '%s'.\n", status.Relay)
			}
		}
	}
}

func repoPermission(cfg Config, pool *nostr.RelayPool) {

	targetPubKey, err := gitnostr.ResolveHexPubKey(os.Args[4])
	if err != nil {
		log.Fatal(err)
	}

	permJson, err := json.Marshal(protocol.RepositoryPermission{
		RepositoryName: os.Args[3],
		TargetPubKey:   targetPubKey,
		Permission:     os.Args[5],
	})

	if err != nil {
		log.Fatal("permission marshal :", err)
	}

	var tags nostr.Tags
	_, statuses, err := pool.PublishEvent(&nostr.Event{
		CreatedAt: time.Now(),
		Kind:      protocol.KindRepositoryPermission,
		Tags:      tags,
		Content:   string(permJson),
	})
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	publishSuccess := false

	for {
		select {
		case <-ctx.Done():
			if !publishSuccess {
				fmt.Printf("permission was not published")
				os.Exit(1)
			}
			return
		case status := <-statuses:
			switch status.Status {
			case nostr.PublishStatusSent:
				publishSuccess = true
				fmt.Printf("published permission to '%s'.\n", status.Relay)
			case nostr.PublishStatusFailed:
				fmt.Printf("failed to publish permission to '%s'.\n", status.Relay)
			case nostr.PublishStatusSucceeded:
				publishSuccess = true
				fmt.Printf("published permission to '%s'.\n", status.Relay)
			}
		}
	}

}

func repoClone(cfg Config, pool *nostr.RelayPool) {

	repoParam := os.Args[3]
	// steve@localhost:public

	split := strings.SplitN(repoParam, ":", 2)

	name := split[0]
	repoName := split[1]

	identifier, err := gitnostr.ResolveHexPubKey(name)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Query for both legacy (kind 51) and NIP-34 (kind 30617) events
	_, subchan := pool.Sub(nostr.Filters{{Kinds: []int{protocol.KindRepository, protocol.KindRepositoryNIP34}, Authors: []string{identifier}}})

	var pubKey string
	var repository protocol.Repository

	for {
		select {
		case <-ctx.Done():
			if pubKey != "" {
				log.Println("git", "clone", repository.GitSshBase+":"+pubKey+"/"+repoName)
				cmd := exec.Command("git", "clone", repository.GitSshBase+":"+pubKey+"/"+repoName)
				cmd.Stdout = os.Stdout
				cmd.Stdin = os.Stdin
				cmd.Stderr = os.Stderr
				err := cmd.Run()
				if err != nil {
					log.Fatal(err)
				}
			} else {
				log.Fatal("Repo not found")
			}

			return
		case event := <-subchan:
			// Handle NIP-34 events (kind 30617) - data is in tags, not content
			if event.Event.Kind == protocol.KindRepositoryNIP34 {
				// Extract repository name from "d" tag
				var foundRepoName string
				for _, tag := range event.Event.Tags {
					if len(tag) >= 2 && tag[0] == "d" {
						foundRepoName = tag[1]
						break
					}
				}
				if foundRepoName == repoName {
					// Try to parse legacy JSON from content for GitSshBase
					var checkRepo protocol.Repository
					if event.Event.Content != "" {
						err := json.Unmarshal([]byte(event.Event.Content), &checkRepo)
						if err == nil {
							checkRepo.RepositoryName = foundRepoName
							repository = checkRepo
						} else {
							// Create minimal repo from tags
							repository = protocol.Repository{
								RepositoryName: foundRepoName,
								PublicRead:     true,  // Default for NIP-34
								PublicWrite:    false, // Default for NIP-34
							}
						}
					} else {
						repository = protocol.Repository{
							RepositoryName: foundRepoName,
							PublicRead:     true,
							PublicWrite:    false,
						}
					}
					pubKey = event.Event.PubKey
				}
			} else {
				// Legacy kind 51 - parse from JSON content
				var checkRepo protocol.Repository
				err := json.Unmarshal([]byte(event.Event.Content), &checkRepo)
				if err != nil {
					log.Println("Failed to parse repository.")
					continue
				}
				if checkRepo.RepositoryName == repoName {
					repository = checkRepo
					pubKey = event.Event.PubKey
				}
			}
		}
	}
}
