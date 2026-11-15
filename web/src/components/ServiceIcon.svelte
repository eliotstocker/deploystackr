<script module lang="ts">
  type IconifySearchResponse = {
    icons?: string[];
    collections?: Record<string, { palette?: boolean }>;
  };

  const resolvedCache = new Map<string, string | null>();
  const inflightCache = new Map<string, Promise<string | null>>();

  function iconifySvgUrl(prefix: string, name: string): string {
    return `https://api.iconify.design/${encodeURIComponent(prefix)}/${encodeURIComponent(name)}.svg`;
  }

  function pickSearchResult(data: IconifySearchResponse): string | null {
    const icons = data.icons || [];
    if(icons.length < 1) {
      return null;
    }
    const collections = data.collections || {};
    const mono = icons.find(id => {
      const [prefix, name = ''] = id.split(':', 2);
      return collections[prefix]?.palette === false && !name.endsWith('-wordmark');
    });
    const chosen = mono || icons[0];
    const [prefix, name] = chosen.split(':', 2);
    if(!prefix || !name) {
      return null;
    }
    return iconifySvgUrl(prefix, name);
  }

  function searchIconify(query: string): Promise<string | null> {
    const url = `https://api.iconify.design/search?query=${encodeURIComponent(query)}&limit=10`;
    return fetch(url)
      .then(res => res.ok ? res.json() : null)
      .then((data: IconifySearchResponse | null) => data ? pickSearchResult(data) : null)
      .catch(() => null);
  }

  function tryQueries(queries: string[], index: number): Promise<string | null> {
    if(index >= queries.length) {
      return Promise.resolve(null);
    }
    const raw = queries[index].trim().toLowerCase();
    if(raw.includes(':')) {
      const [prefix, name] = raw.split(':', 2);
      if(!prefix || !name) {
        return tryQueries(queries, index + 1);
      }
      const directUrl = iconifySvgUrl(prefix, name.replace(/[_\s]+/g, '-'));
      return fetch(directUrl)
        .then(res => res.ok ? directUrl : tryQueries(queries, index + 1))
        .catch(() => tryQueries(queries, index + 1));
    }

    return searchIconify(raw).then(url => url || tryQueries(queries, index + 1));
  }

  function resolveOnlineIcon(iconKey: string, serviceKey: string): Promise<string | null> {
    const iconNorm = iconKey.trim().toLowerCase();
    const serviceNorm = serviceKey.trim().toLowerCase();
    const cacheKey = `${iconNorm}|${serviceNorm}`;
    if(resolvedCache.has(cacheKey)) {
      return Promise.resolve(resolvedCache.get(cacheKey) ?? null);
    }
    const existing = inflightCache.get(cacheKey);
    if(existing) {
      return existing;
    }

    const queries = [iconNorm, serviceNorm].filter((q, i, arr) => q && arr.indexOf(q) === i);
    if(queries.length < 1) {
      resolvedCache.set(cacheKey, null);
      return Promise.resolve(null);
    }

    const req = tryQueries(queries, 0).then(url => {
      resolvedCache.set(cacheKey, url);
      inflightCache.delete(cacheKey);
      return url;
    });
    inflightCache.set(cacheKey, req);
    return req;
  }
</script>

<script lang="ts">
  type Props = {
    icon?: string;
    name?: string;
    size?: number;
  };

  let { icon = '', name = '', size = 20 }: Props = $props();

  const deploystackrRaw = '<path d="M 44 28 C 42 16 37 7 30 5 C 27 13 26 23 28 34 Z" /><path d="M 16 62 C 19 44 32 28 54 27 C 68 26 77 34 81 40 C 84 42.5 89 45 94 47.5 C 95 48.5 94 50 92 51 C 86 52.5 78 54 70 54.5 C 54 55.5 38 56 16 62 Z" /><path d="M 16 62 C 38 56 54 55.5 70 54.5 C 78 54 86 52.5 92 51 C 90 53.5 84 55.5 78 57 C 71 59.5 63 65 58 75 C 56 79 53 79 54 73 C 55 66 52 64 45 66 C 34 69 24 68 16 62 Z" opacity="0.6" />';
  const fallbackPath = 'M12 2L2 7l10 5 10-5-10-5zm0 10.5L3.82 8 2 8.91l10 5 10-5-.91-.45L12 12.5zm0 5L3.82 13 2 13.91l10 5 10-5-.91-.45L12 17.5z';

  const normalize = (k: string) => k.trim().toLowerCase().replace(/[-_\s]/g, '');

  let isRawSvg = $derived(icon.trim().startsWith('<svg'));
  let isUrl = $derived(icon.startsWith('http://') || icon.startsWith('https://') || icon.startsWith('/') || icon.startsWith('data:'));
  let isDeployStackr = $derived.by(() => {
    const iconNorm = normalize(icon);
    const nameNorm = normalize(name);
    const target = iconNorm || nameNorm;
    return target === 'deploystackr' || target === 'dolphin';
  });

  let onlineIconUrl = $state<string | null>(null);

  $effect(() => {
    if(isRawSvg || isUrl || isDeployStackr) {
      onlineIconUrl = null;
      return;
    }

    const cacheKey = `${icon.trim().toLowerCase()}|${name.trim().toLowerCase()}`;
    if(resolvedCache.has(cacheKey)) {
      onlineIconUrl = resolvedCache.get(cacheKey) ?? null;
      return;
    }

    let cancelled = false;
    onlineIconUrl = null;
    resolveOnlineIcon(icon, name).then(url => {
      if(!cancelled) {
        onlineIconUrl = url;
      }
    });

    return () => {
      cancelled = true;
    };
  });
</script>

{#if isRawSvg}
  <span class="raw-svg monochrome-svg" style="width: {size}px; height: {size}px;">
    {@html icon}
  </span>
{:else if isUrl}
  <span
    class="monochrome-logo-mask"
    style="width: {size}px; height: {size}px; -webkit-mask-image: url('{icon}'); mask-image: url('{icon}');"
    title={name}
  ></span>
{:else if isDeployStackr}
  <svg
    width={size}
    height={size}
    viewBox="0 0 100 100"
    fill="currentColor"
    class="custom-svg-icon"
  >
    {@html deploystackrRaw}
  </svg>
{:else if onlineIconUrl}
  <span
    class="monochrome-logo-mask"
    style="width: {size}px; height: {size}px; -webkit-mask-image: url('{onlineIconUrl}'); mask-image: url('{onlineIconUrl}');"
    title={name}
  ></span>
{:else}
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="currentColor"
    class="custom-svg-icon"
  >
    <path d={fallbackPath} />
  </svg>
{/if}

<style>
  .raw-svg {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    color: currentColor;
  }

  .monochrome-svg :global(svg) {
    width: 100%;
    height: 100%;
  }

  .monochrome-svg :global(path),
  .monochrome-svg :global(circle),
  .monochrome-svg :global(polygon),
  .monochrome-svg :global(rect) {
    fill: currentColor !important;
  }

  .monochrome-logo-mask {
    display: inline-block;
    vertical-align: middle;
    flex-shrink: 0;
    background-color: currentColor;
    -webkit-mask-size: contain;
    mask-size: contain;
    -webkit-mask-repeat: no-repeat;
    mask-repeat: no-repeat;
    -webkit-mask-position: center;
    mask-position: center;
  }

  .custom-svg-icon {
    display: inline-block;
    vertical-align: middle;
    flex-shrink: 0;
    color: currentColor;
  }
</style>
