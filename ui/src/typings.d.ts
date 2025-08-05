declare module '*.png';
declare module '*.jpg';
declare module '*.jpeg';
declare module '*.gif';
declare module '*.svg';
declare module '*.css';
declare module '*.wav';
declare module '*.mp3';
declare module '*.m4a';
declare module '*.rdf';
declare module '*.ttl';
declare module '*.pdf';

// Type declaration for react-cytoscapejs
declare module 'react-cytoscapejs' {
  import { Component } from 'react';
  import { Core, ElementDefinition, Stylesheet } from 'cytoscape';

  interface CytoscapeComponentProps {
    elements?: ElementDefinition[] | Record<string, unknown>[];
    style?: React.CSSProperties;
    stylesheet?: Stylesheet[];
    layout?: object;
    cy?: (cy: Core) => void;
    [key: string]: unknown;
  }

  export default class CytoscapeComponent extends Component<CytoscapeComponentProps> {}
}
