export interface Broker {
    id: number;
    name: string;
    listens: any;
    flows: string;
    workflowId: number;
    status: number;
    used: boolean;
    clients: string[] | string;
}