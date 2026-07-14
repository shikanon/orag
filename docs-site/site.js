document.querySelectorAll('[data-copy]').forEach((button) => {
  button.addEventListener('click', async () => {
    try {
      await navigator.clipboard.writeText(button.dataset.copy);
    } catch {
      // Clipboard access can be unavailable in local or embedded previews.
    }
    const label = button.querySelector('.copy-label');
    label.textContent = 'Copied';
    window.setTimeout(() => { label.textContent = 'Copy'; }, 1400);
  });
});
