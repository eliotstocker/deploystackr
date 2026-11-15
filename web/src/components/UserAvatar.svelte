<script lang="ts">
  interface Props {
    name?: string;
    email?: string;
    size?: number;
  }

  let { name, email, size = 16 }: Props = $props();

  let imgFailed = $state(false);

  // If email is available and not a mock/local domain, try loading unavatar
  let avatarUrl = $derived(
    email && !email.endsWith('.local')
      ? `https://unavatar.io/${encodeURIComponent(email)}?fallback=false`
      : null
  );

  let initials = $derived(getInitials(name, email));
  let bgColor = $derived(getAvatarColor(name || email || ''));

  function getInitials(n?: string, e?: string): string {
    if (n && n.trim()) {
      const parts = n.trim().split(/\s+/);
      if (parts.length >= 2) {
        return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
      }
      return n.slice(0, 2).toUpperCase();
    }
    if (e && e.trim()) {
      return e.slice(0, 2).toUpperCase();
    }
    return 'CI';
  }

  function getAvatarColor(str: string): string {
    if (!str) return '#64748b';
    let hash = 0;
    for (let i = 0; i < str.length; i++) {
      hash = str.charCodeAt(i) + ((hash << 5) - hash);
    }
    const colors = [
      '#0284c7', '#0d9488', '#10b981', '#06b6d4',
      '#3b82f6', '#475569', '#334155', '#64748b', '#1e293b'
    ];
    return colors[Math.abs(hash) % colors.length];
  }
</script>

<div
  class="user-avatar-wrap"
  role="tooltip"
  aria-label="{name || 'Automated'}{email ? ` (${email})` : ''}"
>
  {#if avatarUrl && !imgFailed}
    <img
      src={avatarUrl}
      alt={name || 'User'}
      class="avatar-img"
      style="width: {size}px; height: {size}px;"
      onerror={() => (imgFailed = true)}
    />
  {:else}
    <span
      class="avatar-initials"
      style="width: {size}px; height: {size}px; font-size: {Math.max(8, Math.round(size * 0.5))}px; background-color: {bgColor};"
    >
      {initials}
    </span>
  {/if}

  <div class="avatar-tooltip">
    <span class="tooltip-name">{name || 'Automated Deployment'}</span>
    {#if email}
      <span class="tooltip-email">{email}</span>
    {/if}
  </div>
</div>

<style>
  .user-avatar-wrap {
    position: relative;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    cursor: default;
    border-radius: 50%;
  }

  .avatar-img {
    border-radius: 50%;
    object-fit: cover;
    display: block;
  }

  .avatar-initials {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border-radius: 50%;
    color: #ffffff;
    font-weight: 700;
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
    letter-spacing: -0.02em;
    line-height: 1;
    user-select: none;
  }

  .avatar-tooltip {
    position: absolute;
    bottom: calc(100% + 7px);
    left: 50%;
    transform: translateX(-50%) translateY(4px);
    background: #0f172a;
    color: #f8fafc;
    padding: 0.35rem 0.6rem;
    border-radius: 6px;
    font-size: 0.72rem;
    white-space: nowrap;
    pointer-events: none;
    opacity: 0;
    visibility: hidden;
    transition: opacity 0.15s ease, transform 0.15s ease;
    z-index: 1000;
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    align-items: center;
    border: 1px solid rgba(255, 255, 255, 0.15);
  }

  .avatar-tooltip::after {
    content: '';
    position: absolute;
    top: 100%;
    left: 50%;
    transform: translateX(-50%);
    border-width: 4px;
    border-style: solid;
    border-color: #0f172a transparent transparent transparent;
  }

  .user-avatar-wrap:hover .avatar-tooltip {
    opacity: 1;
    visibility: visible;
    transform: translateX(-50%) translateY(0);
  }

  .tooltip-name {
    font-weight: 600;
    color: #ffffff;
    font-size: 0.72rem;
  }

  .tooltip-email {
    font-size: 0.65rem;
    color: #94a3b8;
    font-family: var(--font-mono, monospace);
  }
</style>
