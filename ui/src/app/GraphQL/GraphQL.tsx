import { AppContext } from '@app/AppLayout/AppLayout';
import * as React from 'react';

interface IGraphQLFrameProps {
  url: string;
}

const GraphQLFrame: React.FunctionComponent<IGraphQLFrameProps> = (props) => (
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
      src={props.url}
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

const GraphQL: React.FunctionComponent = () => (
  <AppContext.Consumer>
    {(value) => {
      let url = '/api';
      if (value !== null) {
        const [activeNode] = value.activeNodeState;
        if (activeNode !== '') {
          url = `/api?proxyTo=${activeNode}`;
        }
      }
      return GraphQLFrame({ url: url });
    }}
  </AppContext.Consumer>
);

export { GraphQL };
