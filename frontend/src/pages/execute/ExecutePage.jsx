// ExecutePage — list↔detail router for flowruns. focusEntity.execute can
// pre-open a specific run.
//
// ExecutePage —— flowrun list↔detail router；focusEntity/onConsumeFocusEntity
// 由 AppShell 经 props 传入，pages 层零 app 依赖。

import { useEffect, useState } from "react";
import { AnimatePresence, motion } from "framer-motion";
import { useFlowRun } from "../../api/flowruns.js";
import { ExecuteOverview } from "./ui/ExecuteOverview.jsx";
import { FlowRunDetail } from "@entities/flowrun";
import { slideUp, fadeIn } from "../../motion/tokens.js";

export function ExecutePage({ focusEntity, onConsumeFocusEntity, onOpenChat }) {
  const [openRunId, setOpenRunId] = useState(null);
  const focusId = focusEntity?.execute;

  // Probe and consume incoming focusId
  const { data: probe } = useFlowRun(focusId && !openRunId ? focusId : null);
  useEffect(() => {
    if (focusId && !openRunId && probe) {
      setOpenRunId(focusId);
      onConsumeFocusEntity("execute");
    }
  }, [focusId, openRunId, probe, onConsumeFocusEntity]);

  return (
    <AnimatePresence mode="wait" initial={false}>
      {openRunId ? (
        <motion.div key={`run-${openRunId}`} {...slideUp} style={{ height: "100%" }}>
          <FlowRunDetail runId={openRunId} onBack={() => setOpenRunId(null)} onOpenChat={onOpenChat} />
        </motion.div>
      ) : (
        <motion.div key="list" {...fadeIn} style={{ height: "100%" }}>
          <ExecuteOverview onOpen={(fr) => setOpenRunId(fr.id)} />
        </motion.div>
      )}
    </AnimatePresence>
  );
}
