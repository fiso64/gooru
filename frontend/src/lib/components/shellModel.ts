export type ShellTagLike = {
  name?: string;
  tag?: string;
  namespace?: string;
  value?: string;
  count?: number;
};

export type ShellSavedSearch = {
  id: string;
  name: string;
  query: string;
};

export type ShellCommonTag = {
  tag: string;
  count: number;
};
