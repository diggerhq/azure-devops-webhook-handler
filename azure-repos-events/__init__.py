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

    resource = body["resource"]

    if "repository" in resource:
        repository = resource["repository"]
        project_name = repository["project"]["name"]
        branch_ref = resource["sourceRefName"]
    if "pullRequest" in resource:
        repository = resource["pullRequest"]["repository"]
        project_name = repository["project"]["name"]
        branch_ref = resource["pullRequest"]["sourceRefName"]


    organisation_url = body["resourceContainers"]["account"]["baseUrl"]

    # Call the Azure DevOps Pipeline
    run_azure_devops_pipeline(body, os.getenv("AZURE_TOKEN"), organisation_url, project_name, branch_ref)

    return functions.HttpResponse(
        "Request processed",
        status_code=200
    )


def run_azure_devops_pipeline(context: Dict, auth_token: str, organization_url: str, project_name: str, branch_ref: str) -> None:
    personal_access_token = os.getenv("AZURE_DEVOPS_EXT_PAT")

    # Create a connection to the org
    credentials = BasicAuthentication('', personal_access_token)
    connection = Connection(base_url=organization_url, creds=credentials)

    # Get a client (the "core" client provides access to projects, teams, etc)
    pipelines_client = connection.clients_v7_1.get_pipelines_client()

    # Get list of pipelines for the specified project
    pipelines_list = pipelines_client.list_pipelines(project=project_name)

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
    context_json = json.dumps(context)

    # Prepare the parameters for the pipeline run
    run_parameters = {
        "variables": {
            "AZURE_CONTEXT": {
                "is_secret": False,
                "value": context_json
            },
            "AZURE_TOKEN": {
                "is_secret": True,
                "value": auth_token
            },
            "checkout_branch_ref": {
                "is_secret": False,
                "value": branch_ref
            }
        }
    }

    # Run the pipeline
    pipeline_run = pipelines_client.run_pipeline(run_parameters, project_name, pipeline_id)

    # Log the pipeline run ID
    logging.info(f"Pipeline run started with ID: {pipeline_run.id}")
