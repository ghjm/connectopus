import * as React from 'react';
import { NavLink, useLocation, useNavigate } from 'react-router-dom';
import {
  Nav,
  NavList,
  NavItem,
  NavExpandable,
  Page,
  PageSidebar,
  SkipToContent,
  Flex,
  Label,
  FlexItem,
  PageSidebarBody,
  Masthead,
  MastheadBrand,
  MastheadContent,
  MastheadMain,
  MastheadToggle,
  PageToggleButton,
  Toolbar,
  ToolbarContent,
  ToolbarGroup,
  ToolbarItem,
} from '@patternfly/react-core';
import { Dropdown, DropdownItem, DropdownList, MenuToggle } from '@patternfly/react-core';
import { routes, IAppRoute, IAppRouteGroup } from '@app/routes';
import logo from '@app/images/connectopus.png';
import nodeLogo from '@app/images/node.png';
import { Client, createClient, Provider, useQuery, cacheExchange, fetchExchange } from 'urql';
import { createContext, useEffect } from 'react';
import GitHubButton from 'react-github-btn';
import './AppLayout.css';

interface IAppLayout {
  children: React.ReactNode;
}

interface IAppContext {
  myNodeState: [string, React.Dispatch<React.SetStateAction<string>>];
  activeNodeState: [string, React.Dispatch<React.SetStateAction<string>>];
}

interface IAppContent extends IAppLayout, IAppContext {
  mainClient: Client;
}

const AppContext = createContext<IAppContext | null>(null);

const statusQuery = `{
  status {
    name
    nodes {
      name
      addr
    }
  }
}`;

const AppLayoutContent: React.FunctionComponent<IAppContent> = ({
  myNodeState,
  activeNodeState,
  mainClient,
  children,
}) => {
  const [myNode, setMyNode] = myNodeState;
  const [activeNode, setActiveNode] = activeNodeState;

  const [contextSelectorSelected, setContextSelectorSelected] = React.useState(activeNode);
  const [isContextSelectorOpen, setIsContextSelectorOpen] = React.useState(false);
  const searchText = '';

  const [result, reexecuteQuery] = useQuery({
    query: statusQuery,
    variables: {},
  });
  useEffect(() => {
    if (result.fetching) return;
    if (!('data' in result)) return;
    if (result.data === undefined) return;
    if (!('status' in result.data)) return;
    if (!('name' in result.data.status)) return;
    const sn = result['data']['status']['name'];
    if (sn !== myNode) {
      setMyNode(sn);
      sessionStorage.setItem('myNode', sn);
    }
    const timerId = setTimeout(() => {
      reexecuteQuery({ requestPolicy: 'network-only' });
    }, 1000);
    return () => clearTimeout(timerId);
  }, [result, reexecuteQuery, myNode, setMyNode]);

  function LogoImg() {
    const navigate = useNavigate();
    function handleClick() {
      void navigate('/');
    }
    return <img src={logo} onClick={handleClick} alt="Connectopus Logo" />;
  }

  const onToggle = () => {
    setIsContextSelectorOpen(!isContextSelectorOpen);
  };

  const onSelect = (_: unknown, value: React.ReactNode) => {
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
      .map((node) => (
        <DropdownItem key={node.name} value={node.name}>
          {`${node.name}`}
        </DropdownItem>
      ));
  };

  const Selector = (
    <Dropdown
      isOpen={isContextSelectorOpen}
      onSelect={onSelect}
      onOpenChange={(isOpen: boolean) => setIsContextSelectorOpen(isOpen)}
      toggle={(toggleRef: React.Ref<HTMLButtonElement>) => (
        <MenuToggle ref={toggleRef} onClick={onToggle} isExpanded={isContextSelectorOpen}>
          {contextSelectorSelected}
        </MenuToggle>
      )}
    >
      <DropdownList>{getContextSelectors()}</DropdownList>
    </Dropdown>
  );

  const Header = (
    <Masthead
      display={{ default: 'inline' }}
      style={
        {
          '--pf-v6-c-masthead--BackgroundColor': 'var(--pf-t--color--black)',
          '--pf-v6-c-masthead--BorderColor': 'var(--pf-t--color--gray--80)',
          '--pf-v6-c-page-toggle-button--Color': 'white !important',
          '--pf-v6-c-page-toggle-button--hover--Color': '#f0f0f0 !important',
          '--pf-v6-c-page-toggle-button--focus--Color': 'white !important',
          '--pf-v6-c-page-toggle-button--active--Color': '#e0e0e0 !important',
        } as React.CSSProperties
      }
    >
      <MastheadMain>
        <MastheadToggle>
          <PageToggleButton
            variant="plain"
            aria-label="Global navigation"
            style={
              {
                color: 'white !important',
                '--pf-v6-c-button--Color': 'white !important',
                '--pf-v6-c-button--hover--Color': 'white !important',
                '--pf-v6-c-button--focus--Color': 'white !important',
                '--pf-v6-c-button--active--Color': 'white !important',
              } as React.CSSProperties
            }
          >
            <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor" style={{ color: 'white' }}>
              <path d="M1 3h14v2H1V3zm0 4h14v2H1V7zm0 4h14v2H1v-2z" />
            </svg>
          </PageToggleButton>
        </MastheadToggle>
        <MastheadBrand>
          <LogoImg />
        </MastheadBrand>
      </MastheadMain>
      <MastheadContent>
        <Toolbar id="masthead-toolbar">
          <ToolbarContent>
            <ToolbarGroup align={{ default: 'alignEnd' }}>
              <ToolbarItem>{Selector}</ToolbarItem>
            </ToolbarGroup>
          </ToolbarContent>
        </Toolbar>
      </MastheadContent>
    </Masthead>
  );

  const location = useLocation();

  const renderNavItem = (route: IAppRoute, index: number) => (
    <NavItem key={`${route.label}-${index}`} id={`${route.label}-${index}`} isActive={route.path === location.pathname}>
      <NavLink to={route.path}>{route.label}</NavLink>
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

  const NavFooter = (
    <Flex direction={{ default: 'column' }} cellSpacing={20}>
      <FlexItem alignSelf={{ default: 'alignSelfCenter' }}>
        <Label
          color="grey"
          icon={<img src={nodeLogo} alt={'logo'} width={20} />}
          isCompact={true}
          href={'https://github.com/ghjm/connectopus'}
          style={{ backgroundColor: '#8A8D90' }}
        >
          View on GitHub
        </Label>
      </FlexItem>
      <FlexItem alignSelf={{ default: 'alignSelfCenter' }}>
        <Flex>
          <FlexItem>
            <GitHubButton
              href="https://github.com/ghjm/connectopus/discussions"
              aria-label="Discuss ghjm/connectopus on GitHub"
            >
              Discuss
            </GitHubButton>
          </FlexItem>
          <FlexItem>
            <GitHubButton
              href="https://github.com/ghjm/connectopus"
              data-icon="octicon-star"
              aria-label="Star ghjm/connectopus on GitHub"
            >
              Star
            </GitHubButton>
          </FlexItem>
          <FlexItem>
            <GitHubButton
              href="https://github.com/ghjm/connectopus/issues"
              data-icon="octicon-issue-opened"
              aria-label="Issue ghjm/connectopus on GitHub"
            >
              Issue
            </GitHubButton>
          </FlexItem>
        </Flex>
      </FlexItem>
    </Flex>
  );

  const Navigation = (
    <Flex direction={{ default: 'column' }} flexWrap={{ default: 'nowrap' }} height="100%">
      <FlexItem grow={{ default: 'grow' }} height="100%">
        <Nav id="nav-primary-simple">
          <NavList id="nav-list-simple">
            {routes.map(
              (route, idx) => route.label && (!route.routes ? renderNavItem(route, idx) : renderNavGroup(route, idx)),
            )}
          </NavList>
        </Nav>
      </FlexItem>
      <FlexItem className="pf-u-px-md pf-u-px-lg-on-xl pf-u-color-light-200 pf-u-font-size-sm">{NavFooter}</FlexItem>
    </Flex>
  );

  const Sidebar = (
    <PageSidebar>
      <PageSidebarBody>{Navigation}</PageSidebarBody>
    </PageSidebar>
  );

  const pageId = 'primary-app-container';

  const PageSkipToContent = (
    <SkipToContent
      onClick={(event) => {
        event.preventDefault();
        const primaryContentContainer = document.getElementById(pageId);
        if (primaryContentContainer) primaryContentContainer.focus();
      }}
      href={`#${pageId}`}
    >
      Skip to Content
    </SkipToContent>
  );
  return (
    <Page
      mainContainerId={pageId}
      masthead={Header}
      sidebar={Sidebar}
      skipToContent={PageSkipToContent}
      isContentFilled
      isManagedSidebar={true}
      style={{ height: '100vh', width: '100%' }}
    >
      <Provider value={mainClient}>{children}</Provider>
    </Page>
  );
};

const urlFromActiveNode = (myNode, activeNode: string): string => {
  if (activeNode === '' || activeNode === myNode) {
    return '/query';
  } else {
    return `/proxy/${activeNode}/query`;
  }
};

const getInitialActiveNode = (): string => {
  return sessionStorage.getItem('activeNode') || '';
};

const getInitialMyNode = (): string => {
  return sessionStorage.getItem('myNode') || '';
};

const AppLayout: React.FunctionComponent<IAppLayout> = ({ children }) => {
  const [activeNode, setActiveNode] = React.useState(getInitialActiveNode());
  const [myNode, setMyNode] = React.useState(getInitialMyNode());
  const [client, setClient] = React.useState<Client>(
    createClient({ url: urlFromActiveNode(myNode, activeNode), exchanges: [cacheExchange, fetchExchange] }),
  );
  React.useEffect(() => {
    setClient(createClient({ url: urlFromActiveNode(myNode, activeNode), exchanges: [cacheExchange, fetchExchange] }));
  }, [myNode, activeNode]);
  const [topClient, setTopClient] = React.useState<Client>(
    createClient({ url: urlFromActiveNode(myNode, activeNode), exchanges: [cacheExchange, fetchExchange] }),
  );
  React.useEffect(() => {
    setTopClient(createClient({ url: '/query', exchanges: [cacheExchange, fetchExchange] }));
  }, []);

  return (
    <AppContext.Provider value={{ myNodeState: [myNode, setMyNode], activeNodeState: [activeNode, setActiveNode] }}>
      <Provider value={topClient}>
        <AppLayoutContent
          myNodeState={[myNode, setMyNode]}
          activeNodeState={[activeNode, setActiveNode]}
          mainClient={client}
        >
          {children}
        </AppLayoutContent>
      </Provider>
    </AppContext.Provider>
  );
};

export { AppLayout, AppContext };
