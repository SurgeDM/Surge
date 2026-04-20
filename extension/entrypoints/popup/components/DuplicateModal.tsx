import { createSignal, onMount, onCleanup } from 'solid-js';

export default function DuplicateModal() {
  const [visible, setVisible] = createSignal(false);
  const [filename, setFilename] = createSignal('');
  const [pendingId, setPendingId] = createSignal('');

  const closeAndSend = async (type: 'confirmDuplicate' | 'skipDuplicate') => {
    const id = pendingId();
    setVisible(false);
    setPendingId('');
    if (id) {
      await browser.runtime.sendMessage({ type, id });
    }
  };

  const handleConfirm = async () => { await closeAndSend('confirmDuplicate'); };
  const handleSkip = async () => { await closeAndSend('skipDuplicate'); };

  let confirmBtnRef: HTMLButtonElement | undefined;

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const onPrompt = (msg: any) => {
    if (msg.type === 'promptDuplicate') {
      setPendingId(msg.id);
      setFilename(msg.filename);
      setVisible(true);
      setTimeout(() => confirmBtnRef?.focus(), 100);
    }
  };

  const onKey = (e: KeyboardEvent) => {
    if (e.key === 'Escape' && visible()) handleSkip();
  };

  onMount(async () => {
    browser.runtime.onMessage.addListener(onPrompt);
    document.addEventListener('keydown', onKey);
    const res = await browser.runtime.sendMessage({ type: 'getPendingDuplicates' })
      .catch(() => null) as { duplicates?: { id: string; filename: string }[] } | null;
    const first = res?.duplicates?.[0];
    if (first) {
      setPendingId(first.id);
      setFilename(first.filename);
      setVisible(true);
      setTimeout(() => confirmBtnRef?.focus(), 100);
    }
  });

  onCleanup(() => {
    browser.runtime.onMessage.removeListener(onPrompt);
    document.removeEventListener('keydown', onKey);
  });

  return (
    <div
      class={`modal-overlay${visible() ? '' : ' hidden'}`}
      id="duplicateModal"
      role="alertdialog"
      aria-modal="true"
      aria-labelledby="modal-title"
      aria-describedby="modal-message"
    >
      <div class="modal-container">
        <div class="modal-icon" aria-hidden="true">&#x26A0;&#xFE0F;</div>
        <h2 class="modal-title" id="modal-title">Duplicate Download</h2>
        <p class="modal-message" id="modal-message">This file is already being downloaded:</p>
        <p class="modal-filename" id="duplicateFilename">{filename()}</p>
        <div class="modal-actions">
          <button class="modal-btn modal-btn-secondary" id="duplicateSkip" onClick={handleSkip}>Skip</button>
          <button
            ref={confirmBtnRef}
            class="modal-btn modal-btn-primary"
            id="duplicateConfirm"
            onClick={handleConfirm}
          >
            Download Anyway
          </button>
        </div>
      </div>
    </div>
  );
}
