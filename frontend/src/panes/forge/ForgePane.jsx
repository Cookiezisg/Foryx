// ForgePane — owns its own list ↔ detail router. focusEntity from ui
// store can pre-open a specific entity (used by EntityLink + cmdk).
//
// ForgePane —— 自管 list ↔ detail；ui store focusEntity 可预打开实体。

import { useEffect, useState } from "react";
import { AnimatePresence, motion } from "framer-motion";
import { useUIStore } from "../../store/ui.js";
import { useFunction, useHandler, useWorkflow } from "../../api/forge.js";
import { ForgeList } from "./ForgeList.jsx";
import { FunctionDetail } from "./FunctionDetail.jsx";
import { HandlerDetail } from "./HandlerDetail.jsx";
import { WorkflowDetail } from "./WorkflowDetail.jsx";
import { easeOut, slideUp, fadeIn } from "../../motion/tokens.js";

export function ForgePane() {
  const [open, setOpen] = useState(null);
  const consumeFocusEntity = useUIStore((s) => s.consumeFocusEntity);
  const focusId = useUIStore((s) => s.focusEntity.forge);

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
      consumeFocusEntity("forge");
    }
  }, [focusId, open, probeFn.data, probeHd.data, probeWf.data, consumeFocusEntity]);

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
