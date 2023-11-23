import { parse } from "../helper/objectParser";
import {Workflow, WorkflowFile} from "../models/workflow"
import requests from "./requests";

export const WorkflowsService = {
    create(workflow: WorkflowFile) {
        return requests
            .post(`apis/v1/service/workflow/apply`)
            .send(workflow)
            .then(res => res.body as Workflow);
    },
    list() {
        return requests
            .get(`apis/v1/service/workflow/list`)
            .send()
            .then(res => (res.body.workflows as Workflow[]).map(item =>{
                item.key = item.id
                item.file = parse(item.raw_data)
                return item
            }));
    },
}