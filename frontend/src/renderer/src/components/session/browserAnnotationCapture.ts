import { dataURLToFile } from '@/components/ui/fileTransfer'
import type { Attachment } from '@/lib/api/types'
import type { BrowserAnnotation } from '@/lib/messageContext'
import type { PreviewWebviewElement } from './previewWebview'

export interface BrowserAnnotationCapture {
  annotation: BrowserAnnotation
  screenshot?: Attachment
}

export async function captureBrowserAnnotation(
  webview: PreviewWebviewElement,
  uploadAttachment?: (file: File) => Promise<Attachment>,
): Promise<BrowserAnnotationCapture | null> {
  const annotation = await webview.executeJavaScript<BrowserAnnotation | null>(BROWSER_ANNOTATION_CAPTURE_SCRIPT, true)
  if (!annotation) return null
  let screenshot: Attachment | undefined
  if (uploadAttachment) {
    try {
      const image = await webview.capturePage()
      await clearBrowserAnnotationCapture(webview)
      const file = await dataURLToFile(image.toDataURL(), `browser-annotation-${Date.now()}.png`)
      screenshot = await uploadAttachment(file)
    } catch {
      screenshot = undefined
    }
  }
  return {
    annotation: {
      ...annotation,
      ...(screenshot?.id ? { screenshot_attachment_id: screenshot.id } : {}),
    },
    screenshot,
  }
}

export async function clearBrowserAnnotationCapture(webview: PreviewWebviewElement): Promise<void> {
  await webview.executeJavaScript('window.__jazAnnotationCancel?.()', true).catch(() => undefined)
}

export function isBrowserAnnotationCancelled(error: unknown): boolean {
  const message = error instanceof Error ? error.message : String(error)
  return message.includes('JAZ_ANNOTATION_CANCELLED')
}

const BROWSER_ANNOTATION_CAPTURE_SCRIPT = String.raw`
(() => new Promise((resolve, reject) => {
  window.__jazAnnotationCancel?.();
  const doc = document;
  const root = doc.documentElement;
  const cssEscape = (value) => window.CSS?.escape ? window.CSS.escape(value) : String(value).replace(/[^a-zA-Z0-9_-]/g, '\\$&');
  let target = null;

  const highlight = doc.createElement('div');
  const marker = doc.createElement('div');
  const editor = doc.createElement('form');
  const textarea = doc.createElement('textarea');
  const submit = doc.createElement('button');
  const cancel = doc.createElement('button');

  highlight.setAttribute('data-jaz-annotation-ui', 'true');
  marker.setAttribute('data-jaz-annotation-ui', 'true');
  editor.setAttribute('data-jaz-annotation-ui', 'true');
  Object.assign(highlight.style, {
    position: 'fixed',
    zIndex: '2147483646',
    pointerEvents: 'none',
    border: '2px solid #4d9fff',
    background: 'rgba(77,159,255,0.18)',
    boxShadow: '0 0 0 1px rgba(0,0,0,0.45)',
    borderRadius: '3px',
    display: 'none',
  });
  Object.assign(marker.style, {
    position: 'fixed',
    zIndex: '2147483647',
    width: '28px',
    height: '28px',
    borderRadius: '999px',
    display: 'none',
    placeItems: 'center',
    background: '#4d9fff',
    color: 'white',
    font: '600 14px ui-sans-serif, system-ui, sans-serif',
    boxShadow: '0 5px 18px rgba(0,0,0,0.35)',
    transform: 'translate(-50%, -50%)',
  });
  marker.textContent = '1';
  Object.assign(editor.style, {
    position: 'fixed',
    zIndex: '2147483647',
    display: 'none',
    width: '320px',
    padding: '10px',
    borderRadius: '10px',
    background: 'rgba(32,32,32,0.96)',
    color: 'white',
    boxShadow: '0 16px 50px rgba(0,0,0,0.45)',
    font: '13px ui-sans-serif, system-ui, sans-serif',
  });
  Object.assign(textarea.style, {
    boxSizing: 'border-box',
    width: '100%',
    minHeight: '86px',
    resize: 'vertical',
    border: '1px solid rgba(255,255,255,0.18)',
    borderRadius: '8px',
    padding: '8px',
    outline: 'none',
    color: 'white',
    background: 'rgba(255,255,255,0.08)',
    font: '13px ui-sans-serif, system-ui, sans-serif',
  });
  textarea.placeholder = 'Describe these changes...';
  submit.type = 'submit';
  submit.textContent = 'Add';
  cancel.type = 'button';
  cancel.textContent = 'Cancel';
  for (const button of [cancel, submit]) {
    Object.assign(button.style, {
      height: '30px',
      border: '0',
      borderRadius: '999px',
      padding: '0 12px',
      color: 'white',
      background: button === submit ? '#4d9fff' : 'rgba(255,255,255,0.12)',
      font: '600 12px ui-sans-serif, system-ui, sans-serif',
      cursor: 'pointer',
    });
  }
  const row = doc.createElement('div');
  Object.assign(row.style, { display: 'flex', justifyContent: 'flex-end', gap: '8px', marginTop: '8px' });
  row.append(cancel, submit);
  editor.append(textarea, row);
  doc.body.append(highlight, marker, editor);

  let settled = false;
  const cleanup = () => {
    highlight.remove();
    marker.remove();
    editor.remove();
    doc.removeEventListener('pointermove', onMove, true);
    doc.removeEventListener('click', onClick, true);
    doc.removeEventListener('keydown', onKey, true);
    delete window.__jazAnnotationCancel;
  };

  const rejectCancelled = () => {
    if (settled) {
      cleanup();
      return;
    }
    settled = true;
    cleanup();
    reject(new Error('JAZ_ANNOTATION_CANCELLED'));
  };
  window.__jazAnnotationCancel = rejectCancelled;

  function targetAt(event) {
    const el = doc.elementFromPoint(event.clientX, event.clientY);
    if (!el || el.closest('[data-jaz-annotation-ui="true"]')) return null;
    return el;
  }

  function updateHighlight(el) {
    if (!el) {
      highlight.style.display = 'none';
      return;
    }
    const rect = el.getBoundingClientRect();
    Object.assign(highlight.style, {
      display: 'block',
      left: rect.left + 'px',
      top: rect.top + 'px',
      width: rect.width + 'px',
      height: rect.height + 'px',
    });
  }

  function onMove(event) {
    if (editor.style.display === 'block') return;
    target = targetAt(event);
    updateHighlight(target);
  }

  function onClick(event) {
    if (event.target?.closest?.('[data-jaz-annotation-ui="true"]')) return;
    const el = targetAt(event);
    if (!el) return;
    event.preventDefault();
    event.stopPropagation();
    target = el;
    updateHighlight(el);
    marker.style.display = 'grid';
    marker.style.left = event.clientX + 'px';
    marker.style.top = event.clientY + 'px';
    const left = Math.min(Math.max(12, event.clientX - 24), window.innerWidth - 332);
    const top = Math.min(Math.max(12, event.clientY + 18), window.innerHeight - 150);
    editor.style.display = 'block';
    editor.style.left = left + 'px';
    editor.style.top = top + 'px';
    textarea.focus();
  }

  function onKey(event) {
    if (event.key === 'Escape') {
      event.preventDefault();
      rejectCancelled();
    }
  }

  function nthOfType(el) {
    let n = 1;
    let prev = el.previousElementSibling;
    while (prev) {
      if (prev.localName === el.localName) n++;
      prev = prev.previousElementSibling;
    }
    return n;
  }

  function selectorFor(el) {
    const parts = [];
    let node = el;
    while (node && node.nodeType === 1 && node !== root) {
      let part = node.localName;
      if (node.id) {
        part += '#' + cssEscape(node.id);
        parts.unshift(part);
        break;
      }
      const testID = node.getAttribute('data-testid');
      if (testID) {
        part += '[data-testid=' + JSON.stringify(testID) + ']';
      } else {
        const className = Array.from(node.classList || []).filter(Boolean).slice(0, 2).map((name) => '.' + cssEscape(name)).join('');
        part += className;
        part += ':nth-of-type(' + nthOfType(node) + ')';
      }
      parts.unshift(part);
      node = node.parentElement;
    }
    return parts.join(' > ');
  }

  function pathFor(el) {
    const parts = [];
    let node = el;
    while (node && node.nodeType === 1 && node !== root) {
      parts.unshift(node.localName);
      node = node.parentElement;
    }
    return parts.join(' > ');
  }

  function textFor(el) {
    return (el.innerText || el.textContent || '').replace(/\s+/g, ' ').trim().slice(0, 500);
  }

  cancel.addEventListener('click', (event) => {
    event.preventDefault();
    rejectCancelled();
  });
  editor.addEventListener('submit', (event) => {
    event.preventDefault();
    const comment = textarea.value.trim();
    if (!target || !comment) return;
    settled = true;
    editor.style.display = 'none';
    const rect = target.getBoundingClientRect();
    resolve({
      url: location.href,
      frame: window.top === window ? 'top document' : 'embedded frame',
      target: textFor(target),
      selector: selectorFor(target),
      path: pathFor(target),
      node_position: { x: Math.round(rect.left + rect.width / 2), y: Math.round(rect.top + rect.height / 2) },
      viewport: { width: window.innerWidth, height: window.innerHeight },
      requested_changes: comment,
      comment,
    });
  });

  doc.addEventListener('pointermove', onMove, true);
  doc.addEventListener('click', onClick, true);
  doc.addEventListener('keydown', onKey, true);
}))()
`
