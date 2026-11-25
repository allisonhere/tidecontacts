# Licensing clearance checklist

Status: proposed; do not treat this checklist as permission to distribute or relicense code.

## Existing rights

- [ ] Confirm copyright ownership of TideContacts and authority to license it.
- [ ] Review the complete commit and pull request history for third-party contributions.
- [ ] Obtain explicit permission from firecat53 for existing Nix packaging and CI contributions (commits 3245fc73c61552dcc23e4e527e641c73ddb05aa9 and 0217a9c8a1f06727b9459613db4df6d3b66aa9b5), covering both AGPL and commercial relicensing as appropriate.
- [ ] Confirm the rights and intended license for Ripple; no license was identified at its repository root in the initial review.
- [ ] Confirm licenses at pinned dependency revisions (TideUI and TideMail repository metadata currently identifies MIT; this is not a pinned-version audit).
- [ ] Inventory transitive dependencies and notices; check redistribution requirements.
- [ ] Document any copyright assignment or license to MYLIVELAB LLC.

## Future contributions and release

- [ ] Have counsel review a contributor license agreement permitting commercial relicensing, while allowing contributors to retain copyright if desired.
- [ ] Establish contributor assent records and a review gate before accepting covered contributions.
- [ ] Review AGPL obligations for the TUI, TideMail plugin, future API and professional components.
- [ ] Adopt AGPL-3.0-only using the official unmodified license text after clearance.
- [ ] Update README, SPDX metadata, packaging and notices.
- [ ] Review proprietary EULA, privacy, licensing and support terms before selling the professional edition.

## Suggested contributor outreach (draft, not sent)

Hi firecat53 — thank you again for the Nix packaging and CI work on TideContacts. I'm planning to license the community project under AGPLv3 while retaining the option to offer separate commercial licenses. Would you be open to granting permission for your existing contributions to be included under both arrangements? I'd like to document the permission properly and can share the proposed contributor agreement for review. No change to your rights is assumed without your explicit agreement.
