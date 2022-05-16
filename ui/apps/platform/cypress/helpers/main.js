import * as api from '../constants/apiEndpoints';
import { url } from '../constants/DashboardPage';
import navSelectors from '../selectors/navigation';

import { visit } from './visit';

// visit helpers

export function visitMainDashboardFromLeftNav() {
    cy.intercept('GET', api.risks.riskyDeployments).as('riskyDeployments');

    cy.get(`${navSelectors.navLinks}:contains("Dashboard")`).click();

    cy.wait('@riskyDeployments');
    cy.get('h1:contains("Dashboard")');
}

export function visitMainDashboard() {
    cy.intercept('GET', api.risks.riskyDeployments).as('riskyDeployments');

    visit(url);

    cy.wait('@riskyDeployments');
    cy.get('h1:contains("Dashboard")');
}

export function visitMainDashboardViaRedirectFromUrl(redirectFromUrl) {
    cy.intercept('GET', api.risks.riskyDeployments).as('riskyDeployments');

    visit(redirectFromUrl);

    cy.wait('@riskyDeployments');
    cy.location('pathname').should('eq', url);
    cy.get('h1:contains("Dashboard")');
}
