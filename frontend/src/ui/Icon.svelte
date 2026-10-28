<!-- 아이콘 하나(icons.ts). 단추에는 글자 라벨이나 aria-label이 따로 있으므로 읽기 프로그램에는 숨긴다. -->
<script lang="ts">
  import { ICONS, type IconName } from './icons'

  let { name, size = 16 }: { name: IconName; size?: number } = $props()
</script>

<svg
  class="icon"
  width={size}
  height={size}
  viewBox="0 0 24 24"
  fill="none"
  stroke="currentColor"
  stroke-width="2"
  stroke-linecap="round"
  stroke-linejoin="round"
  aria-hidden="true">{@html ICONS[name]}</svg
>

<style>
  .icon {
    flex: none;
  }
</style>
