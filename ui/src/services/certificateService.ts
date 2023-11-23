import requests from "./requests";
import { Certificate } from "../models/certificate"

export const CertService = {
    create(cert: any) {
        return requests
            .post(`apis/v1/service/certificate/create`)
            .send(cert)
            .then(res => res.body as Certificate);
    },
    list() {
        return requests
            .get(`apis/v1/service/certificate/list`)
            .send()
            .then(res => (res.body.certificates as Certificate[])?.map(item => {
                item.key = item.id + ""
                return item
            }));
    },
}