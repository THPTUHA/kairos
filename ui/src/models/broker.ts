export interface Broker {
    id: number;
    name: string;
    listens: string;
    flows: string;
    workflowId: number;
    status: number;
    used: boolean;
    clients: string[] | string;
}