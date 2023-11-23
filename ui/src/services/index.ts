import { CertService } from "./certificateService";
import { ChannelService } from "./channelService";
import { ClientService } from "./clientService";
import { UserService } from "./userService";
import { WorkflowsService } from "./workflowService";

interface Services {
    workflows: typeof WorkflowsService;
    clients:  typeof ClientService;
    users: typeof UserService;
    channels: typeof ChannelService;
    certs: typeof CertService;
}

export const services: Services = {
    workflows: WorkflowsService,
    clients: ClientService,
    users: UserService,
    channels: ChannelService,
    certs: CertService,
};
