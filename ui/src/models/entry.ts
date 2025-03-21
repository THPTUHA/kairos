
export interface Point {
    id: number;
    name: string;
    type: number;
}


export interface Entry {
    id: number;
    flow_id: number;
    status: number;
    timestamp: number;
    src: Point;
    dst: Point;
    outgoing: boolean;
    requestSize: number;
    responseSize: number;
    elapsedTime: number;
    workflow: {
        name: string,
        id: number,
    };
    cmd: number;
    payload: any;
    reply: boolean;
}
