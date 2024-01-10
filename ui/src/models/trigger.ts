export interface Trigger {
    id: number;
    workflow_id: number;
    object_id: number;
    type: string;
    schedule: string;
    input: string;
    name: string;
    status: number;
    trigger_at: number;
    client: string;
  }
  