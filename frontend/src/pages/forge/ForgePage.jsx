// ForgePage — owns its own list ↔ detail router. focusEntity from ui
// store can pre-open a specific entity (used by EntityLink + cmdk).
//
// ForgePage —— 自管 list ↔ detail；focusEntity/onConsumeFocusEntity 由
// AppShell 经 props 传入，pages 层零 app 依赖。

import { useEffect, useState } from "react";
import { AnimatePresence, motion } from "framer-motion";
import { useFunction, useHandler, useWorkflow } from "../../api/forge.js";
import { ForgeList } from "./ui/ForgeList.jsx";
import { FunctionDetail } from "@/panes/forge/FunctionDetail.jsx";
import { HandlerDetail } from "@/panes/forge/HandlerDetail.jsx";
import { WorkflowDetail } from "@/panes/forge/WorkflowDetail.jsx";
import { slideUp, fadeIn } from "../../motion/tokens.js";

export function ForgePage({ focusEntity, onConsumeFocusEntity }) {
  const [open, setOpen] = useState(null);
  const focusId = focusEntity?.forge;

  // Probe each detail endpoint when focusId is set; whichever returns
  // first determines the kind. (Backend has separate /functions /handlers
  // /workflows endpoints, no unified /forges lookup.)
  const probeFn = useFunction(focusId && !open ? focusId : null);
  const probeHd = useHandler(focusId && !open ? focusId : null);
  const probeWf = useWorkflow(focusId && !open ? focusId : null);

  useEffect(() => {
    if (!focusId || open) return;
    let entity = null, kind = null;
    if (probeFn.data) { entity = probeFn.data; kind = "function"; }
    else if (probeHd.data) { entity = probeHd.data; kind = "handler"; }
    else if (probeWf.data) { entity = probeWf.data; kind = "workflow"; }
    if (entity) {
      setOpen({ ...entity, kind });
      onConsumeFocusEntity("forge");
    }
  }, [focusId, open, probeFn.data, probeHd.data, probeWf.data, onConsumeFocusEntity]);

  const close = () => setOpen(null);

  return (
    <AnimatePresence mode="wait" initial={false}>
      {open ? (
        <motion.div key={`detail-${open.kind}-${open.id}`} {...slideUp} style={{ height: "100%" }}>
          {open.kind === "function" && <FunctionDetail forge={open} onBack={close} />}
          {open.kind === "handler"  && <HandlerDetail forge={open} onBack={close} />}
          {open.kind === "workflow" && <WorkflowDetail forge={open} onBack={close} />}
        </motion.div>
      ) : (
        <motion.div key="list" {...fadeIn} style={{ height: "100%" }}>
          <ForgeList onOpen={setOpen} />
        </motion.div>
      )}
    </AnimatePresence>
  );
}
