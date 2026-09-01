<script lang="ts">
  let { size = 17 } = $props<{ size?: number }>();
</script>

<img
  class="gooru-logo"
  src="/gooru-logo.svg"
  alt="gooru"
  style={`height: ${size}px;`}
/>

<style>
  .gooru-logo {
    display: block;
    width: auto;
    aspect-ratio: 944.03 / 247.49;
    object-fit: contain;
  }
</style>
