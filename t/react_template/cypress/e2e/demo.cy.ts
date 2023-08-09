describe('demo end to end test', () => {
  it('load page', () => {
    cy.visit('/');
    cy.contains('Waiting response').should('exist');
  });
});
