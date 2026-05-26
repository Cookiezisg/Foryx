// useGreeting — picks a single greeting per mount, biased by hour and
// whether the user has a recent conv. Memoized so re-renders don't reshuffle.
//
// useGreeting —— 每次 mount 抽一句问候语；凌晨/深夜或早晨各 50% 偏置时间感
// 子集；有最近对话时 50% 偏置续接类；displayName 空时只抽 name-free 子集。

import { useMemo } from "react";
import { GREETINGS } from "./greetings.js";

function pickFrom(arr) {
  return arr[Math.floor(Math.random() * arr.length)];
}

function filterByName(pool, displayName) {
  if (displayName) return pool;
  return pool.filter((g) => !g.text.includes("{name}"));
}

function selectGreeting({ hasRecentConv, displayName }) {
  const hour = new Date().getHours();
  const pool = filterByName(GREETINGS, displayName);

  const tryBucket = (tag, prob) => {
    if (Math.random() < prob) {
      const sub = pool.filter((g) => g.tags.includes(tag));
      if (sub.length) return pickFrom(sub);
    }
    return null;
  };

  let pick = null;
  if (hour >= 22 || hour < 6) pick = tryBucket("G-night", 0.5);
  else if (hour >= 6 && hour < 11) pick = tryBucket("G-morning", 0.5);
  if (!pick && hasRecentConv) pick = tryBucket("E", 0.5);
  if (!pick) pick = pickFrom(pool);

  return pick.text.replaceAll("{name}", displayName || "");
}

export function useGreeting({ hasRecentConv, displayName }) {
  return useMemo(
    () => selectGreeting({ hasRecentConv, displayName }),
    [hasRecentConv, displayName]
  );
}
