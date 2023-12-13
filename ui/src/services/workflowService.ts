import { parse } from "../helper/objectParser";
import {Workflow, WorkflowFile, WorkflowRecord} from "../models/workflow"
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
    detail(id:number){
        return requests
        .get(`apis/v1/service/workflow/${id}/detail`)
        .send()
        .then(res => (res.body.workflow as Workflow));
    },
    drop(id:number){
        return requests
        .post(`apis/v1/service/workflow/${id}/drop`)
        .send()
        .then(res => (res.body as any));
    },
    record(id: number){
        return requests
        .get(`apis/v1/service/workflow/${id}/record`)
        .send()
        .then(res => (res.body.records as WorkflowRecord[]));
    },
    recover(id: number){
        return requests
        .get(`apis/v1/service/workflow/${id}/recover`)
        .send()
        .then(res => (res.body as any));
    }
}