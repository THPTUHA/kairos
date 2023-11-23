import { parse } from "../helper/objectParser";
import requests from "./requests";
import { Client } from "../models/client"

export const ClientService = {
    create(client: any) {
        return requests
            .post(`apis/v1/service/client/create`)
            .send(client)
            .then(res => res.body as Client);
    },
    list() {
        return requests
            .get(`apis/v1/service/client/list`)
            .send()
            .then(res => (res.body.clients as Client[])?.map(item => {
                item.key = item.id + ""
                return item
            }));
    },
}