// Framer Motion shared params per PRD §3.2.
// Importing from one place keeps timings consistent across the app —
// updating a value here updates every motion in the codebase.
//
// Framer Motion 共享参数（PRD §3.2）；改一处影响全局。

export const spring = { type: "spring", stiffness: 280, damping: 28 };
export const easeOut = { duration: 0.22, ease: [0.2, 0.8, 0.2, 1] };
export const easeFast = { duration: 0.12, ease: [0.2, 0.8, 0.2, 1] };

export const fadeIn = {
  initial: { opacity: 0 },
  animate: { opacity: 1 },
  exit: { opacity: 0 },
  transition: easeOut,
};

export const slideUp = {
  initial: { opacity: 0, y: 8 },
  animate: { opacity: 1, y: 0 },
  exit: { opacity: 0, y: 4 },
  transition: easeOut,
};

export const slideDown = {
  initial: { opacity: 0, y: -8 },
  animate: { opacity: 1, y: 0 },
  exit: { opacity: 0, y: -4 },
  transition: easeOut,
};

export const scaleIn = {
  initial: { opacity: 0, scale: 0.96 },
  animate: { opacity: 1, scale: 1 },
  exit: { opacity: 0, scale: 0.98 },
  transition: easeOut,
};
