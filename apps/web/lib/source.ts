import type * as PageTree from "fumadocs-core/page-tree";
import { loader } from "fumadocs-core/source";

import { docs } from "@/.source/server";

export const source = loader({
  baseUrl: "/",
  source: docs.toFumadocsSource(),
});

const tree = source.pageTree;
const isRoot = (node: PageTree.Node): node is PageTree.Folder =>
  node.type === "folder" && node.root === true;
const roots = tree.children.filter(isRoot);
const rest = tree.children.filter((node) => !isRoot(node));

if (roots.length > 0 && rest.length > 0) {
  const generalFolder: PageTree.Folder = {
    children: rest,
    name: "General",
    root: true,
    type: "folder",
  };

  source.pageTree = { ...tree, children: [generalFolder, ...roots] };
}
