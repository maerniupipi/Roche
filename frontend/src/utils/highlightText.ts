/**
 * Highlight helpers used by ContentPopup (chat references drawer / chunk
 * preview). Lives outside the deleted GlobalCommandPalette folder so the
 * chat side keeps working.
 */

const escapeHtml = (value: string): string =>
  value.replace(/[&<>"']/g, (ch) => {
    switch (ch) {
      case '&':
        return '&amp;';
      case '<':
        return '&lt;';
      case '>':
        return '&gt;';
      case '"':
        return '&quot;';
      default:
        return '&#39;';
    }
  });

const escapeRegex = (value: string): string =>
  value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

const buildHighlightRegex = (pattern: string, literal: boolean): RegExp => {
  const source = literal ? escapeRegex(pattern) : pattern;
  return new RegExp(`(${source})`, 'gi');
};

/**
 * Wrap every whitespace-separated literal term from `pattern` inside an
 * HTML `<mark>` tag. Output is HTML-escaped before highlighting so the
 * caller can inject the result directly.
 */
export const highlightText = (content: string, pattern: string): string => {
  const escaped = escapeHtml(content);
  if (!pattern || !pattern.trim()) return escaped;
  const terms = pattern
    .split(/\s+/)
    .map((term) => term.trim())
    .filter(Boolean);
  if (terms.length === 0) return escaped;

  const regex = new RegExp(
    `(${terms.map(escapeRegex).join('|')})`,
    'gi',
  );
  return escaped.replace(regex, '<mark>$1</mark>');
};

/**
 * Treat `pattern` as a (case-insensitive) regex and wrap matches in
 * `<mark>` tags. The content is HTML-escaped before highlighting, so the
 * pattern must operate on the escaped form if it includes HTML-sensitive
 * characters.
 */
export const highlightRegex = (content: string, pattern: string): string => {
  const escaped = escapeHtml(content);
  if (!pattern) return escaped;
  try {
    const regex = buildHighlightRegex(pattern, false);
    return escaped.replace(regex, '<mark>$1</mark>');
  } catch {
    // Fall back to literal highlighting if the user supplied an invalid
    // regex; failing silently is friendlier than throwing from a render
    // path.
    return highlightText(escaped, pattern);
  }
};