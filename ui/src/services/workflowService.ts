import { Task } from "kairos-js";
import { parse } from "../helper/objectParser";
import {Workflow, WorkflowFile, WorkflowRecord} from "../models/workflow"
import requests from "./requests";
import { Broker } from "../models/broker";
import { Trigger } from "../models/trigger";

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
    },
    getObjects(wid: number){
        return requests
        .get(`apis/v1/service/workflow/${wid}/objects`)
        .send()
        .then(res => (res.body as {
            tasks: Task[],
            brokers: Broker[],
        }));
    },
    getTrigger(wid: number, object_id: number,type: string){
        return requests
        .post(`apis/v1/service/workflow/trigger/list`)
        .send({
            workflow_id: wid,
            object_id,
            type,
        })
        .then(res => (res.body.triggers as Trigger[]));
    },
    deleteTrigger(id: number){
        return requests
        .get(`apis/v1/service/workflow/trigger/delete?id=${id}`)
        .send({})
        .then(res => (res.body));
    },
    saveTrigger(trigger: Trigger){
        return requests
        .post(`apis/v1/service/workflow/trigger`)
        .send(trigger)
        .then(res => (res.body));
    }
}