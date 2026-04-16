%global goipath github.com/qubernetic/copia-cli

Name:           copia-cli
Version:        0.0.0
Release:        1%{?dist}
Summary:        CLI for Copia — source control for industrial automation

License:        AGPL-3.0-only
URL:            https://%{goipath}
Source0:        https://%{goipath}/archive/v%{version}/%{name}-%{version}.tar.gz

BuildRequires:  golang >= 1.26

%description
copia-cli brings Copia repositories, issues, pull requests, and other
concepts to the terminal next to where you are already working with
git and your code.

%prep
%autosetup -n %{name}-%{version}

%build
go build -ldflags "-s -w -X %{goipath}/internal/build.Version=%{version} -X %{goipath}/internal/build.Date=$(date -u +%%Y-%%m-%%d)" -o %{name} ./cmd/copia-cli

%install
install -Dpm 0755 %{name} %{buildroot}%{_bindir}/%{name}
install -Dpm 0644 LICENSE %{buildroot}%{_licensedir}/%{name}/LICENSE

%files
%license LICENSE
%{_bindir}/%{name}

%changelog
