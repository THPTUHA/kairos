import Element from "antd/es/skeleton/Element";
import { ColorElement, colorElement, hasSpecialCharacter, hasSpecialCharacter2 } from "./element";

export function serverUrl(): string {
    return process.env.REACT_APP_HTTPSERVER ? process.env.REACT_APP_HTTPSERVER+"/" : "http://nghia.nexta.vn";
}

export function baseUrl(): string {
    return process.env.REACT_APP_BASE_URL ? process.env.REACT_APP_BASE_URL+"/" : "http://kairosweb.badaosuotdoi.com";
}

export function websocketUrl(): string {
    return process.env.REACT_APP_WEBSOCKET ? process.env.REACT_APP_WEBSOCKET: "ws://nghia.nexta.vn";
}

export function uiUrl(uiPath: string): string {
    return baseUrl() + uiPath;
}

export function uiUrlWithParams(uiPath: string, params: string[]): string {
    if (!params.length) {
        return uiUrl(uiPath);
    }
    return baseUrl() + uiPath + '?' + params.join('&');
}

export function apiUrl(apiPath: string): string {
    return `${serverUrl()}${apiPath}`;
}

const sendExp = ["send", "sendu", "sendsync"]

type Element = {
    value: string,
    hasSend: boolean,
    className: string,
}

export function parseBrokerFlows(flows: string) {
    let cond = ""
    try {
        let fl = JSON.parse(flows)
        cond = fl.Condition
    } catch (error) {
        cond = flows
    }
    const regex = /({{.*?}})/g;

    const matches = cond.split(regex).filter(Boolean);
    // console.log({matches, cond})
    if (matches) {
        const resultArray = matches.map((match: any) => {
            const exps = match.split(/({{)|(}})/g).filter(Boolean);
            const arrs: Element[] = []
            for (const exp of exps) {
                let hasSend = false
                const eles = exp.split(/(\s+)/).filter(Boolean);
                for (const e of eles) {
                    arrs.push({
                        value: e,
                        hasSend,
                        className: colorElement(e, hasSend)
                    })
                    if (sendExp.includes(e)) {
                        hasSend = true
                    }
                }
            }
            return arrs
        });
        return resultArray
    }
    return []
}

export function reset(cond: string) {
    const regex = /({{.*?}})/g;

    const matches = cond.split(regex).filter(Boolean);
    if (matches) {
        const resultArray = matches.map((match: any) => {
            const exps = match.split(/({{)|(}})/g).filter(Boolean);
            const arrs: Element[] = []
            for (const exp of exps) {
                let hasSend = false
                const eles = exp.split(/(\s+)/).filter(Boolean);
                for (const e of eles) {
                    arrs.push({
                        value: e,
                        hasSend,
                        className: colorElement(e, hasSend)
                    })
                    if (sendExp.includes(e)) {
                        hasSend = true
                    }
                }
            }
            return arrs
        });
        return resultArray
    }
    return []
}

const varColor = "text-blue-500"
const constColor = "text-amber-500"
const desColor = "text-green-500"
const funcColor = "text-violet-500"
const errColor = "text-red-500"

export function parseCode(code: string) {
    let hasSend = false
    let open = 0, o = 0
    const result = []
    let row: Element[] = []
    let c = ""
    let chunk = ""
    let currentColor = ""
    // for (let i = 0; i < code.length; i++) {
    //     c = code[i]
    //     chunk+=c
    //     const hasDoubleBraces = /\{\{.*?\}\}/.test(chunk);
    //     if(hasDoubleBraces){

    //     }
    // }

    // if(chunk){
    //     row.push({
    //         value: chunk, hasSend: false, className: currentColor
    //     })
    //     chunk= ""
    // }
    // if(row.length){
    //     result.push(row)
    // }
    return reset(code)
}