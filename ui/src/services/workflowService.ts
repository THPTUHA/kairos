import {Workflow} from "../models/workflow"
import requests from "./requests";

export const WorkflowsService = {
    create(workflow: Workflow) {
        return requests
            .post(`apis/v1/service/workflow/apply`)
            .send(workflow)
            .then(res => res.body as Workflow);
    },
}