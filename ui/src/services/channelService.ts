import requests from "./requests";
import { Channel } from "../models/channel"

export const ChannelService = {
    create(channel: any) {
        return requests
            .post(`apis/v1/service/channel/create`)
            .send(channel)
            .then(res => res.body as Channel);
    },
    list() {
        return requests
            .get(`apis/v1/service/channel/list`)
            .send()
            .then(res => (res.body.channels as Channel[])?.map(item => {
                item.key = item.id + ""
                return item
            }));
    },
    delete(channel:  string){
        return requests
            .post(`apis/v1/service/channel/${channel}/delete`)
            .send()
            .then(res => (res.body as any));
    }
}