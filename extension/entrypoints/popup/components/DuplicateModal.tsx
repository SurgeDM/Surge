import { createSignal } from 'solid-js';

export default function DuplicateModal() {
  const [visible, setVisible] = createSignal(false);
  const [filename, setFilename] = createSignal('');
  const [pendingId, setPendingId] = createSignal('');

  // Listen for duplicate prompts from background
  if (typeof chrome !== 'undefined' && chrome.runtime?.onMessage) {
    const handler = (msg: any) => {
      if (msg.type === 'promptDuplicate') {
        setPendingId(msg.id);
        setFilename(msg.filename);
        setVisible(true);
      }
    };
    chrome.runtime.onMessage.addListener(handler);
  }

  const handleConfirm = async () => {
    const id = pendingId();
    setVisible(false);
    setPendingId('');
    if (id) {
      await browser.runtime.sendMessage({ type: 'confirmDuplicate', id });
    }
  };

  const handleSkip = async () => {
    const id = pendingId();
    setVisible(false);
    setPendingId('');
    if (id) {
      await browser.runtime.sendMessage({ type: 'skipDuplicate', id });
    }
  };

  // Close on escape
  if (typeof document !== 'undefined') {
    document.addEventListener('keydown', (e) => {
      if (e.key === 'Escape' && visible()) handleSkip();
    });
  }

  return (
    <div class={`modal-overlay${visible() ? '' : ' hidden'}`} id="duplicateModal">
      <div class="modal-container">
        <div class="modal-icon">&#x26A0;&#xFE0F;</div>
        <h2 class="modal-title">Duplicate Download</h2>
        <p class="modal-message">This file is already being downloaded:</p>
        <p class="modal-filename" id="duplicateFilename">{filename()}</p>
        <div class="modal-actions">
          <button class="modal-btn modal-btn-secondary" id="duplicateSkip" onClick={handleSkip}>Skip</button>
          <button class="modal-btn modal-btn-primary" id="duplicateConfirm" onClick={handleConfirm}>Download Anyway</button>
        </div>
      </div>
    </div>
  );
}
