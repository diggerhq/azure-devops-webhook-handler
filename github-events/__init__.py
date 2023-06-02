import os
import json
import logging
from typing import Dict
import azure
from azure.devops.connection import Connection
from msrest.authentication import BasicAuthentication
from azure import functions

def main(req: azure.functions.HttpRequest) -> azure.functions.HttpResponse:
    body = req.get_json()
    event_type = req.headers.get('X-GitHub-Event')

    # Validate Event Type
    if event_type not in ["issue_comment", "pull_request"]:
        return functions.HttpResponse(
            "Invalid event type",
            status_code=400
        )

    event = body
    full_name = event['repository']['full_name']
    repo_owner, repository_name = full_name.split("/")

    gh_context = {
        "event": event,
        "event_name": event_type,
        "repository": full_name,
        "repository_owner": repo_owner
    }

    # Call the Azure DevOps Pipeline
    run_azure_devops_pipeline(gh_context, os.getenv("GITHUB_TOKEN"), repo_owner, repository_name)

    return functions.HttpResponse(
        "Request processed",
        status_code=200
    )


def run_azure_devops_pipeline(github_context: Dict, github_token: str, github_repo_owner: str, repo_name: str) -> None:
    organization_url = os.getenv("AZURE_DEVOPS_EXT_ORG_URL")
    personal_access_token = os.getenv("AZURE_DEVOPS_EXT_PAT")

    # Create a connection to the org
    credentials = BasicAuthentication('', personal_access_token)
    connection = Connection(base_url=organization_url, creds=credentials)

    # Get a client (the "core" client provides access to projects, teams, etc)
    pipelines_client = connection.clients_v7_1.get_pipelines_client()

    # Get list of pipelines for the specified project
    pipelines_list = pipelines_client.list_pipelines(project=repo_name)

    # Search for the pipeline with the specified name
    pipeline_id = None
    for pipeline in pipelines_list:
        if pipeline.name == 'digger':
            pipeline_id = pipeline.id
            break

    if not pipeline_id:
        logging.error("Pipeline 'digger' not found")
        return

    # Convert dict to JSON string
    gh_context_json = json.dumps(github_context)

    # Prepare the parameters for the pipeline run
    run_parameters = {
        "variables": {
            "GITHUB_CONTEXT": {
                "is_secret": False,
                "value": gh_context_json
            },
            "GITHUB_REPO_OWNER": {
                "is_secret": False,
                "value": github_repo_owner
            },
            "GITHUB_TOKEN": {
                "is_secret": True,
                "value": github_token
            }
        }
    }

    # Run the pipeline
    pipeline_run = pipelines_client.run_pipeline(run_parameters, repo_name, pipeline_id)

    # Log the pipeline run ID
    logging.info(f"Pipeline run started with ID: {pipeline_run.id}")
