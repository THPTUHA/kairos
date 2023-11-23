import requests from "./requests";
import { User } from "../models/user"

export const UserService = {
    get() {
        return requests
            .get(`apis/v1/service/user/info`)
            .then(res => res.body.user as User);
    },
}