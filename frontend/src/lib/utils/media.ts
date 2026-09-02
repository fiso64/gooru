export type ViewerMedia = {
  media_kind: string;
  media_type: string;
  media_urls: {
    content: string;
    preview: string;
  };
};

export function isAnimatedGif(file: ViewerMedia): boolean {
  return file.media_type.trim().toLowerCase() === 'image/gif';
}

export function isImageViewerMedia(file: ViewerMedia): boolean {
  return file.media_kind === 'photo' || file.media_kind === 'gif' || file.media_kind === 'image' || file.media_type.startsWith('image/');
}

export function canUseOriginalInViewer(file: ViewerMedia): boolean {
  return isImageViewerMedia(file) && Boolean(file.media_urls.content);
}

export function viewerImageSource(file: ViewerMedia, preferOriginal: boolean): string {
  if ((preferOriginal && canUseOriginalInViewer(file)) || isAnimatedGif(file)) {
    return file.media_urls.content || file.media_urls.preview;
  }
  return file.media_urls.preview;
}

export function preserveNativeViewerSize(file: ViewerMedia): boolean {
  return isAnimatedGif(file);
}
