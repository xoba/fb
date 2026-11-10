# Package the example catalog tool.
Name:           catalog
Version:        0.1.9
Release:        1%{?dist}
Summary:        Print labels for an in-memory file catalog
License:        MIT
URL:            https://example.com/catalog
Source0:        %{url}/archive/v%{version}/%{name}-%{version}.tar.gz
BuildArch:      noarch
Requires:       python3

%description
A small tool that prints a label for each file in a catalog,
such as notes.md (120 bytes) and letter.eml (240 bytes).

%prep
%autosetup

%install
install -Dm 0755 catalog.py %{buildroot}%{_bindir}/catalog

%files
%license LICENSE
%{_bindir}/catalog

%changelog
* Tue Nov 10 2026 Example Packager <packager@example.com> - 0.1.9-1
- Initial package
