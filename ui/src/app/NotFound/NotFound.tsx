import * as React from 'react';
import { ExclamationTriangleIcon } from '@patternfly/react-icons';
import { PageSection, Button, EmptyState, EmptyStateBody, EmptyStateFooter } from '@patternfly/react-core';
import { useNavigate } from 'react-router-dom';

const NotFound: React.FunctionComponent = () => {
  function GoHomeBtn() {
    const navigate = useNavigate();
    function handleClick() {
      void navigate('/');
    }
    return <Button onClick={handleClick}>Take me home</Button>;
  }

  return (
    <PageSection>
      <EmptyState variant="full" titleText="404 Page not found" icon={ExclamationTriangleIcon} headingLevel="h1">
        <EmptyStateBody>We didn&apos;t find a page that matches the address you navigated to.</EmptyStateBody>
        <EmptyStateFooter>
          <GoHomeBtn />
        </EmptyStateFooter>
      </EmptyState>
    </PageSection>
  );
};

export { NotFound };
