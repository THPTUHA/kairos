import requests from "./requests";

export interface MessageFlow {
    id: number;
    status: number;
    sender_id: number;
    sender_type: number;
    sender_name: string;
    receiver_id: number;
    receiver_type: number;
    receiver_name: string;
    workflow_id: number;
    message: string;
    attempt: number;
    flow: number;
    created_at: number;
    deliver_id: number;
    request_size: number;
    response_size: number;
    cmd: number;
    workflow_name: string;
    start: boolean;
    group: string;
    task_id: number;
    send_at: number;
    receive_at: number;
    task_name: string;
    part: string;
    parent: string;
    begin_part: boolean;
    finish_part: boolean;
    outobject: any;
    broker_group: string;
    tracking: string;
    value: any;
    start_input: string;
    response: any;
  }

export const GraphService = {
    get({ workflow_ids }:
        { workflow_ids: number[] }) {
        return requests
            .post(`apis/v1/service/graph/get`)
            .send({
                workflow_ids: workflow_ids
            })
            .then(res => res.body.workflows as any);
    },
    data({ workflow_ids }:
        { workflow_ids: number[] }) {
        return requests
            .post(`apis/v1/service/graph/data`)
            .send({
                workflow_ids: workflow_ids
            })
            .then(res => res.body.data as any);
    },
    getTimeLine(trigger_id?:string) {
        return requests
            .get(`apis/v1/service/graph/timeline?trigger_id=${trigger_id}`)
            .send()
            .then(res => res.body.data as MessageFlow[]);
    },
    getGroupID(group: string) {
        return requests
            .get(`apis/v1/service/graph/group?group=${group}`)
            .send()
            .then(res => res.body.data as MessageFlow[]);
    },
    getGroupList(group: string, limit: string) {
        return requests
            .get(`apis/v1/service/graph/group/list?group=${group}&limit=${limit}`)
            .send()
            .then(res => res.body.data as MessageFlow[]);
    },
    getParts(query: any){
        return requests
            .post(`apis/v1/service/graph/part`)
            .send(query)
            .then(res => (
                {
                    inputs: res.body.inputs as MessageFlow[],
                    outputs: res.body.outputs as MessageFlow[],
                }
            ));
    }
}