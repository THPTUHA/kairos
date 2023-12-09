import requests from "./requests";

export const RecordService = {
    getClientRecord(client_id: number) {
        return requests
            .get(`apis/v1/service/record/client/${client_id}`)
            .then(res => res.body as any);
    },
    getTaskRecord(task_id: number) {
        return requests
            .get(`apis/v1/service/record/task/${task_id}`)
            .then(res => res.body as any);
    },
    getBrokerRecord(broker_id: number) {
        return requests
            .get(`apis/v1/service/record/broker/${broker_id}`)
            .then(res => res.body as any);
    },
    getMessageRecord() {
        return requests
            .post(`apis/v1/service/record/message_flows`)
            .send({})
            .then(res => res.body.msg_flows as any);
    },
}