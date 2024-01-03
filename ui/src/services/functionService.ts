import requests from "./requests";
import { Function } from "../models/function"

export type DebugPayload = {
    flows: string,
    input: {
        name: string,
        value: {[key:string]:any}
    }[]
}

export const FunctionSerive = {
    create(func: any) {
        return requests
            .post(`apis/v1/service/functions/create`)
            .send(func)
            .then(res => res.body as Function);
    },
    list() {
        return requests
            .get(`apis/v1/service/functions/list`)
            .send()
            .then(res => (res.body.functions as Function[])?.map(item => {
                item.key = item.id + ""
                return item
            }));
    },
    delete(id: number) {
        return requests
            .get(`apis/v1/service/functions/delete?fid=${id}`)
            .send()
            .then();
    },
    debug(p: DebugPayload) {
        return requests
            .post(`apis/v1/service/functions/debug`)
            .send(p)
            .then(res => res.body);
    },
}