package webhook

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// InboundReleasePayload represents a parsed incoming release notification.
type InboundReleasePayload struct {
	Service     string    `json:"service"`
	Version     string    `json:"version"`
	Description string    `json:"description,omitempty"`
	Changelog   string    `json:"changelog,omitempty"`
	ReleasedAt  time.Time `json:"released_at"`
	Repository  string    `json:"repository,omitempty"`
	Sender      string    `json:"sender,omitempty"`
}

// InboundPRMergedPayload represents a parsed incoming pull request merge notification.
type InboundPRMergedPayload struct {
	Service     string    `json:"service,omitempty"`
	Environment string    `json:"environment,omitempty"`
	PRNumber    int       `json:"pr_number"`
	CommitSHA   string    `json:"commit_sha,omitempty"`
	Branch      string    `json:"branch,omitempty"`
	Repository  string    `json:"repository,omitempty"`
	MergedBy    string    `json:"merged_by,omitempty"`
	MergedAt    time.Time `json:"merged_at"`
}

// ParseReleaseRequest extracts release details from GitHub, generic, or Docker registry webhooks.
func ParseReleaseRequest(r *http.Request, body []byte) (*InboundReleasePayload, error) {
	ghEvent := r.Header.Get("X-GitHub-Event")

	// 1. GitHub Release Webhook
	if ghEvent == "release" || strings.Contains(string(body), `"release"`) {
		var gh struct {
			Action  string `json:"action"`
			Release struct {
				TagName string `json:"tag_name"`
				Name    string `json:"name"`
				Body    string `json:"body"`
			} `json:"release"`
			Repository struct {
				Name     string `json:"name"`
				FullName string `json:"full_name"`
			} `json:"repository"`
			Sender struct {
				Login string `json:"login"`
			} `json:"sender"`
		}
		if err := json.Unmarshal(body, &gh); err == nil && gh.Release.TagName != "" {
			return &InboundReleasePayload{
				Service:     gh.Repository.Name,
				Version:     gh.Release.TagName,
				Description: gh.Release.Name,
				Changelog:   gh.Release.Body,
				Repository:  gh.Repository.FullName,
				Sender:      gh.Sender.Login,
				ReleasedAt:  time.Now(),
			}, nil
		}
	}

	// 2. Generic Release Payload
	var generic struct {
		Event       string `json:"event"`
		Service     string `json:"service"`
		Version     string `json:"version"`
		Tag         string `json:"tag"`
		Description string `json:"description"`
		Changelog   string `json:"changelog"`
		Repository  string `json:"repository"`
		User        string `json:"user"`
	}
	if err := json.Unmarshal(body, &generic); err == nil {
		ver := generic.Version
		if ver == "" {
			ver = generic.Tag
		}
		if ver != "" {
			return &InboundReleasePayload{
				Service:     generic.Service,
				Version:     ver,
				Description: generic.Description,
				Changelog:   generic.Changelog,
				Repository:  generic.Repository,
				Sender:      generic.User,
				ReleasedAt:  time.Now(),
			}, nil
		}
	}

	return nil, fmt.Errorf("unable to parse release payload: missing version/tag")
}

// ParsePRMergedRequest extracts PR merge details from GitHub or generic webhook payloads.
func ParsePRMergedRequest(r *http.Request, body []byte) (*InboundPRMergedPayload, error) {
	ghEvent := r.Header.Get("X-GitHub-Event")

	// 1. GitHub Pull Request Webhook
	if ghEvent == "pull_request" || strings.Contains(string(body), `"pull_request"`) {
		var gh struct {
			Action      string `json:"action"`
			PullRequest struct {
				Number   int    `json:"number"`
				Title    string `json:"title"`
				Merged   bool   `json:"merged"`
				MergeSHA string `json:"merge_commit_sha"`
				Base     struct {
					Ref string `json:"ref"`
				} `json:"base"`
				Head struct {
					Ref string `json:"ref"`
				} `json:"head"`
				MergedBy struct {
					Login string `json:"login"`
				} `json:"merged_by"`
			} `json:"pull_request"`
			Repository struct {
				Name     string `json:"name"`
				FullName string `json:"full_name"`
			} `json:"repository"`
		}
		if err := json.Unmarshal(body, &gh); err == nil {
			if !gh.PullRequest.Merged && gh.Action != "closed" {
				return nil, fmt.Errorf("pull request event is not a merge (action: %s, merged: %v)", gh.Action, gh.PullRequest.Merged)
			}
			return &InboundPRMergedPayload{
				Service:    gh.Repository.Name,
				PRNumber:   gh.PullRequest.Number,
				CommitSHA:  gh.PullRequest.MergeSHA,
				Branch:     gh.PullRequest.Head.Ref,
				Repository: gh.Repository.FullName,
				MergedBy:   gh.PullRequest.MergedBy.Login,
				MergedAt:   time.Now(),
			}, nil
		}
	}

	// 2. Generic PR Merged Payload
	var generic struct {
		Event       string `json:"event"`
		Action      string `json:"action"`
		Service     string `json:"service"`
		Environment string `json:"environment"`
		PRNumber    int    `json:"pr_number"`
		Number      int    `json:"number"`
		CommitSHA   string `json:"commit_sha"`
		Branch      string `json:"branch"`
		User        string `json:"user"`
	}
	if err := json.Unmarshal(body, &generic); err == nil {
		prNum := generic.PRNumber
		if prNum == 0 {
			prNum = generic.Number
		}
		if prNum > 0 || generic.Action == "merged" || generic.Event == "pr_merged" {
			return &InboundPRMergedPayload{
				Service:     generic.Service,
				Environment: generic.Environment,
				PRNumber:    prNum,
				CommitSHA:   generic.CommitSHA,
				Branch:      generic.Branch,
				MergedBy:    generic.User,
				MergedAt:    time.Now(),
			}, nil
		}
	}

	return nil, fmt.Errorf("unable to parse PR merged payload: expected pull_request event or pr_number")
}
