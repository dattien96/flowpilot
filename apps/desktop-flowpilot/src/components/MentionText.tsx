import { findMentionSpans } from "@/components/mentionHighlight";

export function MentionText({
  text,
  skillNames = [],
}: {
  text: string;
  skillNames?: string[];
}): React.ReactElement {
  const spans = findMentionSpans(text, skillNames);
  if (spans.length === 0) return <>{text}</>;
  const parts: React.ReactNode[] = [];
  let cursor = 0;
  spans.forEach((span, i) => {
    if (span.start > cursor) parts.push(text.slice(cursor, span.start));
    parts.push(
      <mark key={`${span.kind}-${span.start}-${i}`} className={`mention-token mention-${span.kind}`}>
        {text.slice(span.start, span.end)}
      </mark>,
    );
    cursor = span.end;
  });
  if (cursor < text.length) parts.push(text.slice(cursor));
  return <>{parts}</>;
}
