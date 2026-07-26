// ----------------
// Data definitions
// ----------------

import { interfaceSatisfiesEnum } from "complete-common";
import { z } from "zod";
import { ClientCommand } from "./enums/ClientCommand";
import {
  hsmPhysicalTruthRequestCommand,
  hsmSnapshotRequestCommand,
} from "./researchHSM";

const clientCommandChatData = z
  .object({
    msg: z.string().min(1),
    room: z.string().min(1),
  })
  .strict()
  .readonly();

// eslint-disable-next-line @typescript-eslint/no-empty-object-type
export interface ClientCommandChatData extends z.infer<
  typeof clientCommandChatData
> {}

const clientCommandChatPMData = z
  .object({
    msg: z.string().min(1),
    recipient: z.string().min(1),
  })
  .strict()
  .readonly();

// eslint-disable-next-line @typescript-eslint/no-empty-object-type
export interface ClientCommandChatPMData extends z.infer<
  typeof clientCommandChatPMData
> {}

// -----------
// Collections
// -----------

export interface ClientCommandData {
  [ClientCommand.chat]: ClientCommandChatData;
  [ClientCommand.chatPM]: ClientCommandChatPMData;
  [ClientCommand.researchHSMPhysicalTruthRequest]: z.infer<
    typeof hsmPhysicalTruthRequestCommand
  >;
  [ClientCommand.researchHSMRequest]: z.infer<
    typeof hsmSnapshotRequestCommand
  >;
}

interfaceSatisfiesEnum<ClientCommandData, ClientCommand>();

export const CLIENT_COMMAND_SCHEMAS = {
  [ClientCommand.chat]: clientCommandChatData,
  [ClientCommand.chatPM]: clientCommandChatPMData,
  [ClientCommand.researchHSMPhysicalTruthRequest]:
    hsmPhysicalTruthRequestCommand,
  [ClientCommand.researchHSMRequest]: hsmSnapshotRequestCommand,
} as const satisfies Record<ClientCommand, unknown>;
