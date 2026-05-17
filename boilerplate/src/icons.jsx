/* eslint-disable react/prop-types */
// Inline icon set — lucide-flavored, hand-tuned to 14px feel.
// Stroke 1.6, round caps/joins, currentColor.

const Svg = ({ children, size = 14, className = "icon", ...rest }) => (
  <svg
    xmlns="http://www.w3.org/2000/svg"
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="1.7"
    strokeLinecap="round"
    strokeLinejoin="round"
    className={className}
    {...rest}
  >
    {children}
  </svg>
);

const Icon = {
  Search: (p) => <Svg {...p}><circle cx="11" cy="11" r="7" /><path d="m20 20-3.5-3.5" /></Svg>,
  Plus: (p) => <Svg {...p}><path d="M12 5v14M5 12h14" /></Svg>,
  ChevronRight: (p) => <Svg {...p}><path d="m9 6 6 6-6 6" /></Svg>,
  ChevronDown: (p) => <Svg {...p}><path d="m6 9 6 6 6-6" /></Svg>,
  ChevronUp: (p) => <Svg {...p}><path d="m6 15 6-6 6 6" /></Svg>,
  X: (p) => <Svg {...p}><path d="M18 6 6 18M6 6l12 12" /></Svg>,
  Check: (p) => <Svg {...p}><path d="M20 6 9 17l-5-5" /></Svg>,
  Bell: (p) => <Svg {...p}><path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9" /><path d="M10 21a2 2 0 0 0 4 0" /></Svg>,
  Command: (p) => <Svg {...p}><path d="M18 6a3 3 0 1 0-3 3v6a3 3 0 1 0 3-3H9a3 3 0 1 0 3 3V9a3 3 0 1 0-3-3" /></Svg>,
  Settings: (p) => <Svg {...p}><circle cx="12" cy="12" r="3" /><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3h.1a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8v.1a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1Z" /></Svg>,
  MessageSquare: (p) => <Svg {...p}><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2Z" /></Svg>,
  Hammer: (p) => <Svg {...p}><path d="m15 12-8.5 8.5a2.12 2.12 0 1 1-3-3L12 9" /><path d="M17.6 6.6 22 11l-7 7-3-3 7-7" /><path d="m9 12-2-2" /></Svg>,
  Play: (p) => <Svg {...p}><path d="M6 4v16l14-8Z" /></Svg>,
  Library: (p) => <Svg {...p}><path d="M3 4v16" /><path d="M7 4v16" /><path d="M11 4h6a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-6Z" /></Svg>,
  User: (p) => <Svg {...p}><circle cx="12" cy="8" r="4" /><path d="M4 21a8 8 0 0 1 16 0" /></Svg>,
  Bot: (p) => <Svg {...p}><rect x="3" y="7" width="18" height="13" rx="3" /><path d="M8 12h.01M16 12h.01M12 3v4M9 20h6" /></Svg>,
  Brain: (p) => <Svg {...p}><path d="M12 4a3 3 0 0 0-3 3v1a3 3 0 0 0-2 5 3 3 0 0 0 2 5 3 3 0 0 0 6 0 3 3 0 0 0 2-5 3 3 0 0 0-2-5V7a3 3 0 0 0-3-3Z" /><path d="M12 8v8" /></Svg>,
  Wrench: (p) => <Svg {...p}><path d="M14.7 6.3a4 4 0 0 1 5 5L9 22l-4-4 10.7-11.7Z" /><path d="m15 5 4 4" /></Svg>,
  Code: (p) => <Svg {...p}><path d="m9 18-6-6 6-6M15 6l6 6-6 6" /></Svg>,
  Paperclip: (p) => <Svg {...p}><path d="m21 11-9 9a5 5 0 1 1-7-7L13 5a3 3 0 1 1 4.5 4L9 17a1.5 1.5 0 1 1-2-2l7-7" /></Svg>,
  File: (p) => <Svg {...p}><path d="M14 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9Z" /><path d="M14 3v6h6" /></Svg>,
  FileText: (p) => <Svg {...p}><path d="M14 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9Z" /><path d="M14 3v6h6M8 13h8M8 17h5" /></Svg>,
  Image: (p) => <Svg {...p}><rect x="3" y="3" width="18" height="18" rx="2" /><circle cx="9" cy="9" r="2" /><path d="m21 15-5-5L5 21" /></Svg>,
  Send: (p) => <Svg {...p}><path d="m22 2-7 20-4-9-9-4Z" /><path d="m22 2-11 11" /></Svg>,
  Square: (p) => <Svg {...p}><rect x="6" y="6" width="12" height="12" rx="2" fill="currentColor" stroke="none" /></Svg>,
  Sparkles: (p) => <Svg {...p}><path d="M12 3 13.5 9 19 10.5 13.5 12 12 18 10.5 12 5 10.5 10.5 9 z" /><path d="M19 17v3M17.5 18.5h3" /></Svg>,
  At: (p) => <Svg {...p}><circle cx="12" cy="12" r="4" /><path d="M16 8v5a3 3 0 0 0 6 0v-1a10 10 0 1 0-4 8" /></Svg>,
  Mic: (p) => <Svg {...p}><rect x="9" y="3" width="6" height="12" rx="3" /><path d="M5 11a7 7 0 0 0 14 0M12 18v3" /></Svg>,
  AlertCircle: (p) => <Svg {...p}><circle cx="12" cy="12" r="9" /><path d="M12 8v4M12 16h.01" /></Svg>,
  CheckCircle: (p) => <Svg {...p}><circle cx="12" cy="12" r="9" /><path d="m9 12 2 2 4-4" /></Svg>,
  Clock: (p) => <Svg {...p}><circle cx="12" cy="12" r="9" /><path d="M12 7v5l3 2" /></Svg>,
  Folder: (p) => <Svg {...p}><path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z" /></Svg>,
  Database: (p) => <Svg {...p}><ellipse cx="12" cy="5" rx="9" ry="3" /><path d="M3 5v6c0 1.7 4 3 9 3s9-1.3 9-3V5" /><path d="M3 11v6c0 1.7 4 3 9 3s9-1.3 9-3v-6" /></Svg>,
  Globe: (p) => <Svg {...p}><circle cx="12" cy="12" r="9" /><path d="M3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18" /></Svg>,
  Workflow: (p) => <Svg {...p}><rect x="3" y="3" width="6" height="6" rx="1" /><rect x="15" y="15" width="6" height="6" rx="1" /><rect x="9" y="9" width="6" height="6" rx="1" /></Svg>,
  GitBranch: (p) => <Svg {...p}><circle cx="6" cy="5" r="2" /><circle cx="18" cy="5" r="2" /><circle cx="12" cy="19" r="2" /><path d="M6 7v6a4 4 0 0 0 4 4h2M18 7v2a4 4 0 0 1-4 4h-2" /></Svg>,
  Cpu: (p) => <Svg {...p}><rect x="5" y="5" width="14" height="14" rx="2" /><rect x="9" y="9" width="6" height="6" /><path d="M2 9h3M2 15h3M19 9h3M19 15h3M9 2v3M15 2v3M9 19v3M15 19v3" /></Svg>,
  Zap: (p) => <Svg {...p}><path d="M13 2 3 14h7l-1 8 10-12h-7Z" /></Svg>,
  Layers: (p) => <Svg {...p}><path d="m12 2 10 5-10 5L2 7Z" /><path d="m2 12 10 5 10-5M2 17l10 5 10-5" /></Svg>,
  KeyRound: (p) => <Svg {...p}><circle cx="8" cy="15" r="4" /><path d="M10.85 12.15 21 2l-3 3 2 2-2 2-2-2" /></Svg>,
  Brush: (p) => <Svg {...p}><path d="M9 22a5 5 0 0 1-5-5c0-3 3-5 6-5L20 2l2 2-10 10c0 3-2 6-5 6Z" /></Svg>,
  Eye: (p) => <Svg {...p}><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8S1 12 1 12Z" /><circle cx="12" cy="12" r="3" /></Svg>,
  EyeOff: (p) => <Svg {...p}><path d="M17 17a10 10 0 0 1-5 1c-7 0-11-8-11-8a17 17 0 0 1 4-5M9.9 5A10 10 0 0 1 23 12s-2 4-6 6.5M2 2l20 20" /></Svg>,
  Copy: (p) => <Svg {...p}><rect x="9" y="9" width="11" height="11" rx="2" /><path d="M5 15V5a2 2 0 0 1 2-2h10" /></Svg>,
  Trash: (p) => <Svg {...p}><path d="M3 6h18M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6" /></Svg>,
  Refresh: (p) => <Svg {...p}><path d="M3 12a9 9 0 0 1 15-6.7L21 8M21 3v5h-5M21 12a9 9 0 0 1-15 6.7L3 16M3 21v-5h5" /></Svg>,
  StopCircle: (p) => <Svg {...p}><circle cx="12" cy="12" r="9" /><rect x="9" y="9" width="6" height="6" rx="1" /></Svg>,
  ArrowUp: (p) => <Svg {...p}><path d="M12 19V5M5 12l7-7 7 7" /></Svg>,
  ArrowRight: (p) => <Svg {...p}><path d="M5 12h14M13 5l7 7-7 7" /></Svg>,
  CornerDownLeft: (p) => <Svg {...p}><path d="M9 10 4 15l5 5M20 4v7a4 4 0 0 1-4 4H4" /></Svg>,
  Pin: (p) => <Svg {...p}><path d="M12 17v5M9 10.8V4h6v6.8L18 13H6Z" /></Svg>,
  Filter: (p) => <Svg {...p}><path d="M3 4h18l-7 8v6l-4 2v-8Z" /></Svg>,
  MoreHorizontal: (p) => <Svg {...p}><circle cx="5" cy="12" r="1.5" /><circle cx="12" cy="12" r="1.5" /><circle cx="19" cy="12" r="1.5" /></Svg>,
  Menu: (p) => <Svg {...p}><path d="M3 6h18M3 12h18M3 18h18" /></Svg>,
  Inbox: (p) => <Svg {...p}><path d="M22 12h-6l-2 3h-4l-2-3H2" /><path d="M5 4h14l3 8v6a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2v-6Z" /></Svg>,
  Terminal: (p) => <Svg {...p}><path d="m4 17 6-6-6-6M12 19h8" /></Svg>,
  HelpCircle: (p) => <Svg {...p}><circle cx="12" cy="12" r="9" /><path d="M9.1 9a3 3 0 0 1 5.8 1c0 2-3 3-3 3M12 17h.01" /></Svg>,
  Pause: (p) => <Svg {...p}><rect x="6" y="4" width="4" height="16" rx="1" /><rect x="14" y="4" width="4" height="16" rx="1" /></Svg>,
  Activity: (p) => <Svg {...p}><path d="M22 12h-4l-3 9L9 3l-3 9H2" /></Svg>,
  Server: (p) => <Svg {...p}><rect x="3" y="4" width="18" height="6" rx="1" /><rect x="3" y="14" width="18" height="6" rx="1" /><path d="M6 7h.01M6 17h.01" /></Svg>,
  Boxes: (p) => <Svg {...p}><path d="M2.97 12.92A2 2 0 0 0 2 14.63v3.24a2 2 0 0 0 .97 1.71l3 1.8a2 2 0 0 0 2.06 0L11 19.7V13l-4-2.4a2 2 0 0 0-2.07 0Z" /><path d="m7 16.5-4.74-2.8M7 16.5v5.17M7 16.5l4.74-2.8" /><path d="M2.97 4.92A2 2 0 0 0 2 6.63v3.24a2 2 0 0 0 .97 1.7l3 1.81a2 2 0 0 0 2.06 0L11 11.7V5l-4-2.4a2 2 0 0 0-2.07 0Z" /><path d="m13.97 12.92-1 .58a2 2 0 0 0-.97 1.71v3.24a2 2 0 0 0 .97 1.71l3 1.8a2 2 0 0 0 2.06 0l3-1.8a2 2 0 0 0 .97-1.7v-3.25a2 2 0 0 0-.97-1.7l-3-1.81a2 2 0 0 0-2.06 0Z" /></Svg>,
  Package: (p) => <Svg {...p}><path d="m7.5 4.27 9 5.15M21 8 12 13 3 8M3 8v8l9 5 9-5V8L12 3Z" /></Svg>,
  ListChecks: (p) => <Svg {...p}><path d="m3 17 2 2 4-4M3 7l2 2 4-4M13 6h8M13 12h8M13 18h8" /></Svg>,
  Edit: (p) => <Svg {...p}><path d="M16 4l4 4-12 12H4v-4Z" /></Svg>,
  Sun: (p) => <Svg {...p}><circle cx="12" cy="12" r="4" /><path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41" /></Svg>,
  Moon: (p) => <Svg {...p}><path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8Z" /></Svg>,
};

window.Icon = Icon;
