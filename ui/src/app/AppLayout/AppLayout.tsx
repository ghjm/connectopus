import * as React from 'react';
import { NavLink, useLocation, useHistory } from 'react-router-dom';
import {
  Nav,
  NavList,
  NavItem,
  NavExpandable,
  Page,
  PageHeader,
  PageSidebar,
  SkipToContent,
  PageHeaderTools,
  ContextSelector,
  ContextSelectorItem,
} from '@patternfly/react-core';
import { routes, IAppRoute, IAppRouteGroup } from '@app/routes';
import logo from '@app/images/connectopus.png';
import { Client, createClient, Provider, useQuery } from 'urql';
import { createContext, useEffect } from 'react';

interface IAppLayout {
  children: React.ReactNode;
}

interface IAppContext {
  activeNodeState: [string, React.Dispatch<React.SetStateAction<string>>];
}

interface IAppContent extends IAppLayout, IAppContext {
  mainClient: Client;
}

const AppContext = createContext<IAppContext | null>(null);

const statusQuery = `{
  status {
    nodes {
      name
      addr
    }
  }
}`;

const AppLayoutContent: React.FunctionComponent<IAppContent> = ({ activeNodeState, mainClient, children }) => {
  const [activeNode, setActiveNode] = activeNodeState;
  const [isNavOpen, setIsNavOpen] = React.useState(true);
  const [isMobileView, setIsMobileView] = React.useState(true);
  const [isNavOpenMobile, setIsNavOpenMobile] = React.useState(false);
  const [contextSelectorSelected, setContextSelectorSelected] = React.useState(activeNode);
  const [isContextSelectorOpen, setIsContextSelectorOpen] = React.useState(false);
  const [searchText, setSearchText] = React.useState('');
  const [searchValue, setSearchValue] = React.useState('');
  const [result, reexecuteQuery] = useQuery({
    query: statusQuery,
  });
  useEffect(() => {
    if (result.fetching) return;
    const timerId = setTimeout(() => {
      reexecuteQuery({ requestPolicy: 'network-only' });
    }, 1000);
    return () => clearTimeout(timerId);
  }, [result.fetching, reexecuteQuery]);

  const onNavToggleMobile = () => {
    setIsNavOpenMobile(!isNavOpenMobile);
  };
  const navMobileClose = () => {
    setIsNavOpenMobile(false);
  };
  const onNavToggle = () => {
    setIsNavOpen(!isNavOpen);
  };
  const onPageResize = (props: { mobileView: boolean; windowSize: number }) => {
    setIsMobileView(props.mobileView);
  };

  function LogoImg() {
    const history = useHistory();
    function handleClick() {
      history.push('/');
    }
    return <img src={logo} onClick={handleClick} alt="Connectopus Logo" />;
  }

  const onToggle = (event: unknown, isOpen: boolean) => {
    setIsContextSelectorOpen(isOpen);
  };

  const onSearchInputChange = (value: string) => {
    setSearchValue(value);
  };

  const onSearchButtonClick = () => {
    setSearchText(searchValue);
  };

  const onSelect = (event: unknown, value: React.ReactNode) => {
    if (value !== undefined && value !== null) {
      const newNode = value.toString();
      setActiveNode(newNode);
      setContextSelectorSelected(newNode);
      sessionStorage.setItem('activeNode', newNode);
      setIsContextSelectorOpen(false);
    }
  };

  const getContextSelectors = () => {
    if (!('data' in result && result.data !== undefined && 'status' in result.data)) {
      return null;
    }
    return result.data['status']['nodes']
      .filter((node) => {
        if (searchText === '') {
          return true;
        }
        return node.name.includes(searchText);
      })
      .map((node) => <ContextSelectorItem key={node.name}>{`${node.name}`}</ContextSelectorItem>);
  };

  const Selector = (
    <ContextSelector
      toggleText={contextSelectorSelected}
      searchInputValue={searchValue}
      isOpen={isContextSelectorOpen}
      onToggle={onToggle}
      onSelect={onSelect}
      onSearchInputChange={onSearchInputChange}
      onSearchButtonClick={onSearchButtonClick}
    >
      {getContextSelectors()}
    </ContextSelector>
  );

  const Header = (
    <PageHeader
      logo={<LogoImg />}
      showNavToggle
      isNavOpen={isNavOpen}
      onNavToggle={isMobileView ? onNavToggleMobile : onNavToggle}
      headerTools={<PageHeaderTools>{Selector}</PageHeaderTools>}
    />
  );

  const location = useLocation();

  const renderNavItem = (route: IAppRoute, index: number) => (
    <NavItem
      key={`${route.label}-${index}`}
      id={`${route.label}-${index}`}
      isActive={route.path === location.pathname}
      onClick={navMobileClose}
    >
      <NavLink exact={route.exact} to={route.path}>
        {route.label}
      </NavLink>
    </NavItem>
  );

  const renderNavGroup = (group: IAppRouteGroup, groupIndex: number) => (
    <NavExpandable
      key={`${group.label}-${groupIndex}`}
      id={`${group.label}-${groupIndex}`}
      title={group.label}
      isActive={group.routes.some((route) => route.path === location.pathname)}
    >
      {group.routes.map((route, idx) => route.label && renderNavItem(route, idx))}
    </NavExpandable>
  );

  const Navigation = (
    <Nav id="nav-primary-simple" theme="dark">
      <NavList id="nav-list-simple">
        {routes.map(
          (route, idx) => route.label && (!route.routes ? renderNavItem(route, idx) : renderNavGroup(route, idx))
        )}
      </NavList>
    </Nav>
  );

  const Sidebar = <PageSidebar theme="dark" nav={Navigation} isNavOpen={isMobileView ? isNavOpenMobile : isNavOpen} />;

  const pageId = 'primary-app-container';

  const PageSkipToContent = (
    <SkipToContent
      onClick={(event) => {
        event.preventDefault();
        const primaryContentContainer = document.getElementById(pageId);
        primaryContentContainer && primaryContentContainer.focus();
      }}
      href={`#${pageId}`}
    >
      Skip to Content
    </SkipToContent>
  );
  return (
    <Page
      mainContainerId={pageId}
      header={Header}
      sidebar={Sidebar}
      onPageResize={onPageResize}
      skipToContent={PageSkipToContent}
    >
      <Provider value={mainClient}>{children}</Provider>
    </Page>
  );
};

const urlFromActiveNode = (activeNode: string): string => {
  if (activeNode === '') {
    return '/query';
  } else {
    return `/proxy/${activeNode}/query`;
  }
};

const getInitialActiveNode = (): string => {
  return sessionStorage.getItem('activeNode') || '';
};

const AppLayout: React.FunctionComponent<IAppLayout> = ({ children }) => {
  const [activeNode, setActiveNode] = React.useState(getInitialActiveNode());
  const [client, setClient] = React.useState<Client>(createClient({ url: urlFromActiveNode(activeNode) }));
  React.useEffect(() => {
    setClient(createClient({ url: urlFromActiveNode(activeNode) }));
  }, [activeNode]);
  const [topClient, setTopClient] = React.useState<Client>(createClient({ url: urlFromActiveNode(activeNode) }));
  React.useEffect(() => {
    setTopClient(createClient({ url: '/query' }));
  }, []);

  return (
    <AppContext.Provider value={{ activeNodeState: [activeNode, setActiveNode] }}>
      <Provider value={topClient}>
        <AppLayoutContent activeNodeState={[activeNode, setActiveNode]} mainClient={client}>
          {children}
        </AppLayoutContent>
      </Provider>
    </AppContext.Provider>
  );
};

export { AppLayout, AppContext };
