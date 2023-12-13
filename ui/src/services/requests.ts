import * as _superagent from 'superagent';
import {SuperAgentRequest} from 'superagent';
import { apiUrl, uiUrlWithParams } from '../helper/base';
const superagentPromise = require('superagent-promise');

const auth = (req: SuperAgentRequest) => {
    req.set('Authorization', `Bearer ${localStorage.getItem('accessToken')}`);
    return req.on('error', handle);
};

const handle = (err: any) => {
    if (err.status === 401 && !document.location.href.includes('home')) {
        document.location.href = uiUrlWithParams('home', []);
    }
};


const superagent: _superagent.SuperAgentStatic = superagentPromise(_superagent, global.Promise);


export default {
    get(url: string) {
        return auth(superagent.get(apiUrl(url)));
    },

    post(url: string) {
        return auth(superagent.post(apiUrl(url)));
    },

}