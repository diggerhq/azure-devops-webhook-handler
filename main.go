package azure_devops_webhook_handler

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/microsoft/azure-devops-go-api/azuredevops"
	"github.com/microsoft/azure-devops-go-api/azuredevops/pipelines"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

type GithubContext struct {
	Event           interface{} `json:"event"`
	EventName       string      `json:"event_name"`
	Repository      string      `json:"repository"`
	RepositoryOwner string      `json:"repository_owner"`
}

type PullRequestEvent struct {
	Action      string      `json:"action"`
	Number      int         `json:"number"`
	PullRequest PullRequest `json:"pull_request"`
	Repository  Repository  `json:"repository"`
}

type PullRequest struct {
	Number int  `json:"number"`
	Merged bool `json:"merged"`
	Base   Base `json:"base"`
}

type IssueCommentEvent struct {
	Comment    Comment    `json:"comment"`
	Issue      Issue      `json:"issue"`
	Repository Repository `json:"repository"`
}

type Base struct {
	Ref string `json:"ref"`
}

type Comment struct {
	Body string `json:"body"`
}

type Issue struct {
	Number int `json:"number"`
}

type Repository struct {
	DefaultBranch string `json:"default_branch"`
	FullName      string `json:"full_name"`
}

func HandleEvent(w http.ResponseWriter, req *http.Request) {
	// Read the request body
	body, err := io.ReadAll(req.Body)
	if err != nil {
		log.Printf("Failed to read request body: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	eventType := req.Header.Get("X-GitHub-Event")
	var event interface{}
	if eventType == "issue_comment" {
		event = IssueCommentEvent{}
		err = json.Unmarshal(body, &event)
		if err != nil {
			log.Printf("Failed to parse request body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	} else if eventType == "pull_request" {
		event = PullRequestEvent{}
		err = json.Unmarshal(body, &event)
		if err != nil {
			log.Printf("Failed to parse request body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	// Create the GitHub context
	var fullName string

	if eventType == "issue_comment" {
		fullName = event.(IssueCommentEvent).Repository.FullName
	} else if eventType == "pull_request" {
		fullName = event.(PullRequestEvent).Repository.FullName
	}
	splitRepositoryName := strings.Split(fullName, "/")
	repoOwner, repositoryName := splitRepositoryName[0], splitRepositoryName[1]
	ghContext := GithubContext{
		Event:           event,
		EventName:       eventType,
		Repository:      repositoryName,
		RepositoryOwner: repoOwner,
	}

	runAzureDevopsPipeline(ghContext, os.Getenv("GITHUB_TOKEN"), repoOwner)

	w.WriteHeader(http.StatusOK)
}

func runAzureDevopsPipeline(githubContext GithubContext, githubToken string, githubRepoOwner string) {
	// Your Azure DevOps organization URL and personal access token
	organizationURL := os.Getenv("AZURE_DEVOPS_EXT_ORG_URL")
	personalAccessToken := os.Getenv("AZURE_DEVOPS_EXT_PAT")

	// Create a new connection using the Azure DevOps Go SDK
	connection := azuredevops.NewPatConnection(organizationURL, personalAccessToken)

	// Create a new pipeline client
	pipelineClient := pipelines.NewClient(context.Background(), connection)

	// Get a list of pipelines in the specified project
	listPipelinesArgs := pipelines.ListPipelinesArgs{
		Project: &githubContext.Repository,
	}
	pipelineList, err := pipelineClient.ListPipelines(context.Background(), listPipelinesArgs)
	if err != nil {
		log.Fatalf("Failed to list pipelines: %v", err)
	}

	// Search for the pipeline with the specified name
	var pipelineID *int
	for _, pipeline := range (*pipelineList).Value {
		if *pipeline.Name == "digger" {
			pipelineID = pipeline.Id
			break
		}
	}

	if pipelineID == nil {
		log.Fatalf("Pipeline '%s' not found", "digger")
	}
	// Convert struct to JSON string
	ghContext, err := json.Marshal(githubContext)
	if err != nil {
		log.Fatalf("Failed to convert struct to JSON: %v", err)
	}

	// Trigger the pipeline run
	runPipelineArgs := pipelines.RunPipelineArgs{
		Project:    &githubContext.Repository,
		PipelineId: pipelineID,
		RunParameters: &pipelines.RunPipelineParameters{
			Variables: &map[string]pipelines.Variable{
				"GITHUB_CONTEXT": {
					IsSecret: BoolAddr(false),
					Value:    StrAddr(string(ghContext)),
				},
				"GITHUB_REPO_OWNER": {
					IsSecret: BoolAddr(false),
					Value:    StrAddr(githubRepoOwner),
				},
				"GITHUB_TOKEN": {
					IsSecret: BoolAddr(true),
					Value:    StrAddr(githubToken),
				},
			},
		},
	}
	pipelineRun, err := pipelineClient.RunPipeline(context.Background(), runPipelineArgs)
	if err != nil {
		log.Fatalf("Failed to run pipeline: %v", err)
	}

	// Print the pipeline run ID
	fmt.Printf("Pipeline run started with ID: %s\n", *pipelineRun.Id)
}

func BoolAddr(b bool) *bool {
	boolVar := b
	return &boolVar
}

func StrAddr(b string) *string {
	strVar := b
	return &strVar
}

func main() {
	http.HandleFunc("/event", HandleEvent)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
