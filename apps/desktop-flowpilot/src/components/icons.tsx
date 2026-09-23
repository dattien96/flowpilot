import React from "react";

// Shared chrome icon set — Devin/VS Code style 16px line icons, single source.
// Every glyph carries explicit width/height so it renders at the right size even
// before styles.css applies (un-sized <svg> defaults to 300x150 and was the
// cause of buttons inflating on first paint).
interface IconProps {
  size?: number;
  className?: string;
}

function icon(
  paths: React.ReactNode,
  viewBox = "0 0 16 16",
): (props: IconProps) => React.ReactElement {
  return function Icon({ size = 16, className }: IconProps): React.ReactElement {
    return (
      <svg
        viewBox={viewBox}
        width={size}
        height={size}
        className={className}
        fill="none"
        stroke="currentColor"
        strokeWidth="1.4"
        strokeLinecap="round"
        strokeLinejoin="round"
        aria-hidden="true"
      >
        {paths}
      </svg>
    );
  };
}

export const PanelLeftIcon = icon(
  <>
    <rect x="2" y="3" width="12" height="10" rx="1.5" />
    <path d="M6.5 3v10" />
  </>,
);

export const PanelRightIcon = icon(
  <>
    <rect x="2" y="3" width="12" height="10" rx="1.5" />
    <path d="M9.5 3v10" />
  </>,
);

export const TerminalIcon = icon(
  <>
    <rect x="2" y="3" width="12" height="10" rx="1.5" />
    <path d="M5 7l2.2 2L5 11" />
    <path d="M8.5 11H11" />
  </>,
);

export const PaperclipIcon = icon(
  <path d="M11.5 7.5 7 12a2.4 2.4 0 0 1-3.4-3.4l5-5a1.6 1.6 0 0 1 2.3 2.3l-5 5" />,
);

export const SendIcon = icon(
  <path d="M8 12.5v-9M4.5 7 8 3.5 11.5 7" strokeWidth="1.6" />,
);

export const InboxIcon = icon(
  <>
    <path d="M3 8.5V11a1.5 1.5 0 0 0 1.5 1.5h7A1.5 1.5 0 0 0 13 11V8.5" />
    <path d="M3.2 8.5 4.7 4.2A1.5 1.5 0 0 1 6.1 3.2h3.8a1.5 1.5 0 0 1 1.4 1l1.5 4.3" />
    <path d="M3 8.5h3l.8 1.2a1.5 1.5 0 0 0 1.2.6h0a1.5 1.5 0 0 0 1.2-.6l.8-1.2h3" />
  </>,
);

export const WarnIcon = icon(
  <>
    <path d="M8 2.3 14.5 13a.6.6 0 0 1-.5.9H2a.6.6 0 0 1-.5-.9L8 2.3z" />
    <path d="M8 6.4v3.2" />
    <circle cx="8" cy="11.4" r="0.7" fill="currentColor" stroke="none" />
  </>,
);

export const GateIcon = icon(
  <>
    <path d="M8 1.8 13.2 5v6L8 14.2 2.8 11V5L8 1.8z" />
    <path d="M5.7 8h4.6" />
  </>,
);

export const CloseIcon = icon(
  <path d="M4.5 4.5l7 7M11.5 4.5l-7 7" />,
);

export const PlusIcon = icon(
  <path d="M8 3.5v9M3.5 8h9" />,
);

export const BoardIcon = icon(
  <>
    <rect x="2.5" y="2.5" width="4.6" height="4.6" rx="1" />
    <rect x="8.9" y="2.5" width="4.6" height="4.6" rx="1" />
    <rect x="2.5" y="8.9" width="4.6" height="4.6" rx="1" />
    <rect x="8.9" y="8.9" width="4.6" height="4.6" rx="1" />
  </>,
);

export const BotIcon = icon(
  <>
    <rect x="3.5" y="6" width="9" height="6.5" rx="1.5" />
    <path d="M8 3.2v2.8" />
    <circle cx="8" cy="2.5" r="0.8" fill="currentColor" stroke="none" />
    <path d="M6 9.2h.9M9.1 9.2h.9" />
  </>,
);

export const MenuIcon = icon(
  <path d="M3 4.5h10M3 8h10M3 11.5h10" />,
);

export const CheckIcon = icon(
  <path d="M3.5 8.5 6.5 11.5 12.5 4.5" strokeWidth="1.7" />,
);

export const SpinnerIcon = icon(
  <path d="M8 2.5a5.5 5.5 0 1 0 5.5 5.5" strokeWidth="1.6" />,
);

export const BanIcon = icon(
  <>
    <circle cx="8" cy="8" r="5.5" />
    <path d="M4.5 4.5l7 7" />
  </>,
);

export const PencilIcon = icon(
  <path d="M9.8 3.2a1.4 1.4 0 0 1 2 2l-7.2 7.2-2.6.6.6-2.6 7.2-7.2z" />,
);

export const MinusIcon = icon(
  <path d="M3.5 8h9" />,
);

export const ArrowRightIcon = icon(
  <path d="M2.5 8h10M8.5 4.5 12 8l-3.5 3.5" />,
);

export const ExternalIcon = icon(
  <path d="M6.5 3.5h6v6M12.5 3.5 7 9M5 5.5H3.5v7h7V11" />,
);

export const CopyIcon = icon(
  <>
    <rect x="5.5" y="5.5" width="8" height="8" rx="1.2" />
    <path d="M10.5 5.5v-2a1.2 1.2 0 0 0-1.2-1.2H3.7A1.2 1.2 0 0 0 2.5 3.5v5.6a1.2 1.2 0 0 0 1.2 1.2h1.8" />
  </>,
);

export const GearIcon = icon(
  <>
    <circle cx="8" cy="8" r="2.2" />
    <path d="M8 1.8v1.9M8 12.3v1.9M1.8 8h1.9M12.3 8h1.9M3.7 3.7l1.3 1.3M11 11l1.3 1.3M12.3 3.7 11 5M5 11l-1.3 1.3" />
  </>,
);

export const ChevronLeftIcon = icon(
  <path d="M9.5 3.5 5 8l4.5 4.5" />,
);

export const ChevronRightIcon = icon(
  <path d="M6.5 3.5 11 8l-4.5 4.5" />,
);

export const GlobeIcon = icon(
  <>
    <circle cx="8" cy="8" r="5.8" />
    <path d="M2.2 8h11.6M8 2.2c1.6 1.6 2.4 3.5 2.4 5.8s-.8 4.2-2.4 5.8c-1.6-1.6-2.4-3.5-2.4-5.8S6.4 3.8 8 2.2z" />
  </>,
);

// Disclosure triangle: points right when collapsed, rotates down when open.
export const DisclosureCaret = ({ open, size = 10 }: { open?: boolean; size?: number }): React.ReactElement => (
  <svg
    viewBox="0 0 10 10"
    width={size}
    height={size}
    aria-hidden="true"
    fill="none"
    style={{ transform: open ? "rotate(90deg)" : undefined, transition: "transform 120ms" }}
  >
    <path d="M3.5 2.5 6.5 5 3.5 7.5" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" />
  </svg>
);

export const CaretIcon = ({ open, size = 10 }: { open?: boolean; size?: number }): React.ReactElement => (
  <svg
    viewBox="0 0 10 10"
    width={size}
    height={size}
    aria-hidden="true"
    fill="currentColor"
    style={{ transform: open ? "rotate(180deg)" : undefined, transition: "transform 120ms" }}
  >
    <path d="M2.5 3.5 5 6.5 7.5 3.5" stroke="currentColor" strokeWidth="1.4" fill="none" strokeLinecap="round" strokeLinejoin="round" />
  </svg>
);
