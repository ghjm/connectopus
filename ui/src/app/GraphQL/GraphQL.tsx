import * as React from 'react';

const GraphQL: React.FunctionComponent = () => (
  <div
    style={{
      position: 'relative',
      top: 0,
      left: 0,
      bottom: 0,
      right: 0,
      width: '100%',
      height: '100%',
      border: 'none',
      margin: 0,
      padding: 0,
      overflow: 'hidden',
    }}
  >
    <iframe
      src="/api"
      style={{
        position: 'relative',
        top: 0,
        left: 0,
        bottom: 0,
        right: 0,
        width: '100%',
        height: '100%',
        border: 'none',
        margin: 0,
        padding: 0,
        overflow: 'hidden',
      }}
    >
      Your browser doesn&apos;t support iframes
    </iframe>
  </div>
);

export { GraphQL };
