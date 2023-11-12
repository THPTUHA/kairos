import { WorkflowsService } from "./workflowService";

interface Services {
    workflows: typeof WorkflowsService;
}

export const services: Services = {
    workflows: WorkflowsService,
};
