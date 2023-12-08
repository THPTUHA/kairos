import requests from "./requests";

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
}