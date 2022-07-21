import * as React from 'react';
import '@patternfly/react-core/dist/styles/base.css';
import { BrowserRouter as Router } from 'react-router-dom';
import { AppLayout } from '@app/AppLayout/AppLayout';
import { AppRoutes } from '@app/routes';
import '@app/app.css';
import { Unauthorized } from '@app/Unauthorized/Unauthorized';
import { NoAgent } from '@app/Unconfigured/NoAgent';
import { NoNode } from '@app/Unconfigured/NoNode';

const App: React.FunctionComponent = () => {
  const serverData = window['__SERVER_DATA__'];
  const pageSel = serverData['page_select'];
  if (pageSel === 'unauthorized') {
    return <Unauthorized />;
  }
  if (pageSel === 'no-agent') {
    return <NoAgent />;
  }
  if (pageSel === 'no-node') {
    return <NoNode />;
  }
  return (
    <Router>
      <AppLayout>
        <AppRoutes />
      </AppLayout>
    </Router>
  );
};

export default App;
