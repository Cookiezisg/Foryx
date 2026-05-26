// Cross-slice bridge for other entity slices to read currentUserId reactively.
// Entity-to-entity imports must go through @x to keep FSD slice boundaries.
export { useSessionStore } from "../model/sessionStore";
